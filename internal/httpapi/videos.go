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
	"socialnet/internal/worker"
)

const maxVideoBytes = 1 << 30 // 1 ГБ

// handleUploadVideo: POST /api/v1/videos (multipart: file, title, description).
// Быстрая проверка — сразу, перекодирование — в фоне. Ответ 202, статус processing.
func (s *Server) handleUploadVideo(w http.ResponseWriter, r *http.Request) {
	if !av.Available() {
		writeError(w, http.StatusServiceUnavailable, "media_disabled", "media processing is not available on this server")
		return
	}
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}
	up, ok := s.receiveUpload(w, r, maxVideoBytes)
	if !ok {
		return
	}
	defer up.cleanup()

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
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "supported: MP4, MOV, MKV, WEBM")
		return
	case err != nil:
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "cannot read the video file")
		return
	}
	v := probe.Video()
	if v == nil || v.Width <= 0 || v.Height <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "file has no video")
		return
	}
	if v.Width > 7680 || v.Height > 4320 {
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "resolution above 8K is not supported")
		return
	}
	if probe.Seconds < 1 || probe.Seconds > worker.MaxVideoSeconds {
		writeError(w, http.StatusUnprocessableEntity, "bad_duration", "video must be 1 second to 30 minutes long")
		return
	}

	title := cleanText(firstNonEmpty(up.Fields["title"],
		strings.TrimSuffix(filepath.Base(up.Filename), filepath.Ext(up.Filename))), 200)
	if title == "" {
		title = "Видео"
	}
	desc := strings.TrimSpace(strings.ToValidUTF8(up.Fields["description"], ""))
	if len([]rune(desc)) > 5000 {
		desc = string([]rune(desc)[:5000])
	}

	id := av.NewID()
	src, err := s.avStore.SourcePath(id)
	if err == nil {
		err = os.Rename(up.Path, src) // исходник ждёт воркера
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.CreateVideo(r.Context(), store.NewVideo{ID: id, Title: title, Description: desc,
		OwnerID: me, SourceSHA: up.SHA, Format: probe.Format}); err != nil {
		_ = os.Remove(src)
		s.internalError(w, r, err)
		return
	}
	video, err := s.store.Video(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, video)
}

// video загружает {vid} с проверкой доступа: необработанное видно только
// владельцу, чужое — по правилам видимости профиля.
func (s *Server) video(w http.ResponseWriter, r *http.Request, ownerOnly bool) (*store.Video, bool) {
	id := strings.ToLower(chi.URLParam(r, "vid"))
	notFound := func() (*store.Video, bool) {
		writeError(w, http.StatusNotFound, "not_found", "video not found")
		return nil, false
	}
	if !validUUID(id) {
		return notFound()
	}
	v, err := s.store.Video(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		return notFound()
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	me := userIDFrom(r.Context())
	if v.OwnerID == me {
		return v, true
	}
	if v.Status != "ready" {
		return notFound()
	}
	owner, err := s.store.UserByID(r.Context(), v.OwnerID)
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	rel, err := s.store.Relationship(r.Context(), me, v.OwnerID)
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if owner.BannedAt != nil || !canSee(owner.ProfileVisibility, false, rel) {
		return notFound()
	}
	if ownerOnly {
		writeError(w, http.StatusForbidden, "forbidden", "not your video")
		return nil, false
	}
	return v, true
}

func (s *Server) handleGetVideo(w http.ResponseWriter, r *http.Request) {
	if v, ok := s.video(w, r, false); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

func (s *Server) handleUserVideos(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	self := target.ID == userIDFrom(r.Context())
	if !canSee(target.ProfileVisibility, self, rel) {
		writeError(w, http.StatusForbidden, "private", "profile is closed")
		return
	}
	items, err := s.store.UserVideos(r.Context(), target.ID, self, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleUpdateVideo(w http.ResponseWriter, r *http.Request) {
	v, ok := s.video(w, r, true)
	if !ok {
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	if req.Title != nil {
		t := cleanText(*req.Title, 200)
		if t == "" {
			errs["title"] = "1-200 characters"
		}
		req.Title = &t
	}
	desc := trimField(errs, "description", req.Description, 5000)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.UpdateVideo(r.Context(), v.ID, req.Title, desc); err != nil {
		s.internalError(w, r, err)
		return
	}
	updated, err := s.store.Video(r.Context(), v.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteVideo(w http.ResponseWriter, r *http.Request) {
	v, ok := s.video(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteVideo(r.Context(), v.ID, userIDFrom(r.Context())); err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.avStore.Delete("videos", v.ID)
	if src, err := s.avStore.SourcePath(v.ID); err == nil {
		_ = os.Remove(src)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleServeVideo: GET /video/{vid}/{file} — плейлисты, сегменты и превью
// для HLS-плеера (hls.js или нативный в Safari). Только готовые видео.
func (s *Server) handleServeVideo(w http.ResponseWriter, r *http.Request) {
	id, file := strings.ToLower(chi.URLParam(r, "vid")), chi.URLParam(r, "file")
	if !validUUID(id) {
		http.NotFound(w, r)
		return
	}
	v, err := s.store.Video(r.Context(), id)
	if err != nil || v.Status != "ready" {
		http.NotFound(w, r)
		return
	}
	var ctype string
	switch {
	case strings.HasSuffix(file, ".m3u8"):
		ctype = "application/vnd.apple.mpegurl"
	case strings.HasSuffix(file, ".ts"):
		ctype = "video/mp2t"
	case file == "poster.jpg":
		ctype = "image/jpeg"
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // hls.js грузит сегменты через fetch
	s.serveAVFile(w, r, "videos", id, file, ctype)
}
