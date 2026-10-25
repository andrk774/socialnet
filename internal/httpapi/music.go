package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/av"
	"socialnet/internal/store"
)

const (
	maxTrackBytes   = 50 << 20 // 50 МБ
	maxTrackSeconds = 20 * 60
)

func (s *Server) handleUploadTrack(w http.ResponseWriter, r *http.Request) {
	if !av.Available() {
		writeError(w, http.StatusServiceUnavailable, "media_disabled", "media processing is not available on this server")
		return
	}
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}
	up, ok := s.receiveUpload(w, r, maxTrackBytes)
	if !ok {
		return
	}
	defer up.cleanup()

	// снятый по жалобе правообладателя файл не загрузить повторно
	blocked, err := s.store.IsBlocked(r.Context(), up.SHA)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if blocked {
		writeError(w, http.StatusUnavailableForLegalReasons, "blocked_content", "this file was removed at the request of the rights holder")
		return
	}

	probe, err := av.ProbeFile(r.Context(), up.Path)
	switch {
	case errors.Is(err, av.ErrUnsupported):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "supported: MP3, OGG, FLAC, WAV, M4A")
		return
	case err != nil:
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "cannot read the audio file")
		return
	}
	if probe.Audio() == nil {
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "file has no audio")
		return
	}
	if probe.Seconds < 1 || probe.Seconds > maxTrackSeconds {
		writeError(w, http.StatusUnprocessableEntity, "bad_duration", "track must be 1 second to 20 minutes long")
		return
	}

	title := cleanText(firstNonEmpty(up.Fields["title"], probe.Tags["title"],
		strings.TrimSuffix(filepath.Base(up.Filename), filepath.Ext(up.Filename))), 200)
	artist := cleanText(firstNonEmpty(up.Fields["artist"], probe.Tags["artist"]), 200)
	if title == "" {
		title = "Без названия"
	}
	if artist == "" {
		artist = "Неизвестный исполнитель"
	}

	release, ok := s.acquireAV(w)
	if !ok {
		return
	}
	defer release()

	id := av.NewID()
	dir, _ := s.avStore.Dir("tracks", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.internalError(w, r, err)
		return
	}
	out, _ := s.avStore.Path("tracks", id, "audio.mp3")
	if err := av.TranscodeAudio(r.Context(), up.Path, out, probe.Format, maxTrackSeconds); err != nil {
		_ = s.avStore.Delete("tracks", id)
		s.log.Warn("transcode audio", "err", err)
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "cannot process the audio file")
		return
	}
	st, err := os.Stat(out)
	if err == nil {
		err = s.store.CreateTrack(r.Context(), store.NewTrack{ID: id, Title: title, Artist: artist,
			UploaderID: me, DurationSec: int(probe.Seconds + 0.5), Size: st.Size(), SourceSHA: up.SHA})
	}
	if err != nil {
		_ = s.avStore.Delete("tracks", id)
		s.internalError(w, r, err)
		return
	}
	track, err := s.store.Track(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, track)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// trackParam проверяет {tid} и загружает трек. Снятый трек — 404.
func (s *Server) trackParam(w http.ResponseWriter, r *http.Request) (*store.Track, bool) {
	id := strings.ToLower(chi.URLParam(r, "tid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return nil, false
	}
	t, err := s.store.Track(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && t.Removed) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return t, true
}

func (s *Server) handleGetTrack(w http.ResponseWriter, r *http.Request) {
	if t, ok := s.trackParam(w, r); ok {
		writeJSON(w, http.StatusOK, t)
	}
}

func (s *Server) handleDeleteTrack(w http.ResponseWriter, r *http.Request) {
	t, ok := s.trackParam(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteTrack(r.Context(), t.ID, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "forbidden", "only the uploader can delete a track")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.avStore.Delete("tracks", t.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleServeAudio: GET /audio/{file} — публичный адрес для <audio>,
// поддерживает Range (перемотка). Снятый трек перестаёт отдаваться сразу.
func (s *Server) handleServeAudio(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.ToLower(chi.URLParam(r, "file")), ".mp3")
	if !validUUID(id) {
		http.NotFound(w, r)
		return
	}
	t, err := s.store.Track(r.Context(), 0, id)
	if err != nil || t.Removed {
		http.NotFound(w, r)
		return
	}
	s.serveAVFile(w, r, "tracks", id, "audio.mp3", "audio/mpeg")
}

func (s *Server) serveAVFile(w http.ResponseWriter, r *http.Request, kind, id, name, ctype string) {
	path, err := s.avStore.Path(kind, id, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	// не immutable: снятый по жалобе контент должен пропасть из кэшей
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// ---------- «Моя музыка» ----------

func (s *Server) handleAddMyMusic(w http.ResponseWriter, r *http.Request) {
	t, ok := s.trackParam(w, r)
	if !ok {
		return
	}
	if err := s.store.AddToMyMusic(r.Context(), userIDFrom(r.Context()), t.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveMyMusic(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(chi.URLParam(r, "tid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return
	}
	if err := s.store.RemoveFromMyMusic(r.Context(), userIDFrom(r.Context()), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Чужая музыка видна по тем же правилам, что и профиль.
func (s *Server) handleUserMusic(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if !canSee(target.ProfileVisibility, target.ID == me, rel) {
		writeError(w, http.StatusForbidden, "private", "profile is closed")
		return
	}
	p := pageParams(r)
	tracks, cursor, err := s.store.MyMusic(r.Context(), me, target.ID, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	resp := listResponse[store.Track]{Items: tracks}
	if len(tracks) == p.Limit {
		resp.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- плейлисты ----------

func (s *Server) playlistTitle(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return "", false
	}
	t := cleanText(req.Title, 100)
	if t == "" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"title": "1-100 characters"})
		return "", false
	}
	return t, true
}

func (s *Server) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	title, ok := s.playlistTitle(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	id, err := s.store.CreatePlaylist(r.Context(), me, title)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	pl, err := s.store.Playlist(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, pl)
}

// playlist загружает плейлист {id} с проверкой видимости профиля владельца.
// own=true — нужен владелец (правка).
func (s *Server) playlist(w http.ResponseWriter, r *http.Request, own bool) (*store.Playlist, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	me := userIDFrom(r.Context())
	pl, err := s.store.Playlist(r.Context(), me, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if pl.OwnerID != me {
		owner, err := s.store.UserByID(r.Context(), pl.OwnerID)
		if err != nil {
			s.internalError(w, r, err)
			return nil, false
		}
		rel, err := s.store.Relationship(r.Context(), me, pl.OwnerID)
		if err != nil {
			s.internalError(w, r, err)
			return nil, false
		}
		if owner.BannedAt != nil || !canSee(owner.ProfileVisibility, false, rel) {
			writeError(w, http.StatusNotFound, "not_found", "playlist not found")
			return nil, false
		}
		if own {
			writeError(w, http.StatusForbidden, "forbidden", "not your playlist")
			return nil, false
		}
	}
	return pl, true
}

func (s *Server) handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	if pl, ok := s.playlist(w, r, false); ok {
		writeJSON(w, http.StatusOK, pl)
	}
}

func (s *Server) handleUserPlaylists(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if !canSee(target.ProfileVisibility, target.ID == userIDFrom(r.Context()), rel) {
		writeError(w, http.StatusForbidden, "private", "profile is closed")
		return
	}
	items, err := s.store.Playlists(r.Context(), target.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRenamePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, ok := s.playlist(w, r, true)
	if !ok {
		return
	}
	title, ok := s.playlistTitle(w, r)
	if !ok {
		return
	}
	if err := s.store.RenamePlaylist(r.Context(), pl.ID, title); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, ok := s.playlist(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeletePlaylist(r.Context(), pl.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddPlaylistTrack(w http.ResponseWriter, r *http.Request) {
	pl, ok := s.playlist(w, r, true)
	if !ok {
		return
	}
	var req struct {
		TrackID string `json:"track_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	id := strings.ToLower(req.TrackID)
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return
	}
	err := s.store.AddPlaylistTrack(r.Context(), pl.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "track not found or playlist is full")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemovePlaylistTrack(w http.ResponseWriter, r *http.Request) {
	pl, ok := s.playlist(w, r, true)
	if !ok {
		return
	}
	id := strings.ToLower(chi.URLParam(r, "tid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "track not found")
		return
	}
	if err := s.store.RemovePlaylistTrack(r.Context(), pl.ID, id); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
