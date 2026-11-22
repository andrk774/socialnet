package httpapi

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"socialnet/internal/av"
	"strconv"
	"strings"
)

func (s *Server) handleUploadVideoMessage(w http.ResponseWriter, r *http.Request) {
	if !av.Available() {
		writeError(w, http.StatusServiceUnavailable, "media_disabled", "media processing is not available on this server")
		return
	}
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}
	up, ok := s.receiveUpload(w, r, 50<<20)
	if !ok {
		return
	}
	defer up.cleanup()
	probe, err := av.ProbeFile(r.Context(), up.Path)
	if errors.Is(err, av.ErrUnsupported) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "supported: MP4, WEBM")
		return
	}
	if err != nil || probe.Video() == nil || probe.Video().Width > 4096 || probe.Video().Height > 4096 || probe.Seconds < 0.5 || probe.Seconds > 60 {
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "video message must be video up to 60 seconds and 4096 pixels")
		return
	}
	release, ok := s.acquireAV(w)
	if !ok {
		return
	}
	defer release()
	id := av.NewID()
	dir, _ := s.avStore.Dir("voices", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.internalError(w, r, err)
		return
	}
	out, _ := s.avStore.Path("voices", id, "720p.mp4")
	if err := av.TranscodeVideoMessage(r.Context(), up.Path, out, probe.Format, 60); err != nil {
		_ = s.avStore.Delete("voices", id)
		writeError(w, http.StatusUnprocessableEntity, "bad_file", "cannot process the video")
		return
	}
	secs := int(probe.Seconds + 0.5)
	if secs < 1 {
		secs = 1
	}
	if err := s.avStore.Commit(r.Context(), "voices", id); err != nil {
		_ = s.avStore.Delete("voices", id)
		s.internalError(w, r, err)
		return
	}
	if err := s.store.CreateVideoMessage(r.Context(), id, me, secs); err != nil {
		_ = s.avStore.Delete("voices", id)
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "duration_sec": secs, "url": "/api/v1/video-messages/" + id})
}

// handleServeVideoMessage: GET /api/v1/voice/{id} — только автору и участникам чата,
// где голосовое отправлено. Плеер получает его через fetch с токеном.
func (s *Server) handleServeVideoMessage(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(chi.URLParam(r, "vid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	ok, err := s.store.VoiceAccess(r.Context(), id, userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store") // личное — не в общие кэши
	s.serveAVFile(w, r, "voices", id, "720p.mp4", "video/mp4")
}
