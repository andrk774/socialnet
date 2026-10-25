package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/media"
	"socialnet/internal/store"
)

// handleUpload: POST /api/v1/media, multipart/form-data, поле "file".
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, media.MaxBytes+(1<<20)) // файл + заголовки формы
	file, _, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "max file size is 10 MiB")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", `multipart field "file" is required`)
		return
	}
	defer func() { _ = file.Close() }()
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll() // временные файлы формы
		}
	}()

	res, err := media.Process(file)
	switch {
	case errors.Is(err, media.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "max file size is 10 MiB")
		return
	case errors.Is(err, media.ErrUnsupported):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "only JPEG, PNG and GIF images are allowed")
		return
	case errors.Is(err, media.ErrTooManyPixel), errors.Is(err, media.ErrCorrupt):
		writeError(w, http.StatusUnprocessableEntity, "bad_image", err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}

	id, err := s.store.CreateMedia(r.Context(), me, res.Mime, res.Width, res.Height, int64(len(res.Full)))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.saveFiles(id, res); err != nil {
		_ = s.store.DeleteMedia(r.Context(), id, me)
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "url": *store.MediaURL(&id, "full"), "thumb_url": *store.MediaURL(&id, "thumb"),
		"width": res.Width, "height": res.Height, "mime": res.Mime,
	})
}

func (s *Server) saveFiles(id string, res *media.Result) error {
	if err := s.files.Save(id, "full", res.Full); err != nil {
		return err
	}
	if err := s.files.Save(id, "thumb", res.Thumb); err != nil {
		_ = s.files.Delete(id)
		return err
	}
	return nil
}

// handleServeMedia: GET /media/{id}/{size}. Публичный адрес для <img>:
// id — случайный UUID, перебрать его невозможно.
func (s *Server) handleServeMedia(w http.ResponseWriter, r *http.Request) {
	id, size := strings.ToLower(chi.URLParam(r, "id")), chi.URLParam(r, "size")
	if !validUUID(id) || (size != "full" && size != "thumb") {
		http.NotFound(w, r)
		return
	}
	info, err := s.store.Media(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if _, remote := s.files.(*media.S3); remote && s.cfg.MediaCDNURL != "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.Redirect(w, r, s.cfg.MediaCDNURL+"/photos/"+id+"/"+size, http.StatusFound)
		return
	}
	f, mod, err := s.files.Open(id, size)
	if errors.Is(err, media.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()

	// Тип берём из базы (мы его сами определили при загрузке), а не угадываем.
	w.Header().Set("Content-Type", info.Mime)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	http.ServeContent(w, r, "", mod, f)
}

func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(chi.URLParam(r, "id"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	err := s.store.DeleteMedia(r.Context(), id, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) { // чужое фото выглядит как несуществующее
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.files.Delete(id); err != nil {
		s.log.Error("delete media files", "err", err, "id", id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetAvatar: PUT /users/me/avatar {"media_id": "<uuid>"} или {"media_id": null}.
func (s *Server) handleSetAvatar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MediaID *string `json:"media_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.MediaID != nil {
		v := strings.ToLower(*req.MediaID)
		if !validUUID(v) {
			writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
				map[string]string{"media_id": "invalid id"})
			return
		}
		req.MediaID = &v
	}
	err := s.store.SetAvatar(r.Context(), userIDFrom(r.Context()), req.MediaID)
	if errors.Is(err, store.ErrBadMedia) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"media_id": "unknown photo or not yours"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.handleMe(w, r)
}
