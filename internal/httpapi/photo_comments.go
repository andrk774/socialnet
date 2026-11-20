package httpapi

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (s *Server) handlePhotoComments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, 404, "not_found", "photo not found")
		return
	}
	allowed, err := s.store.CanReadPhotoComments(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, 404, "not_found", "photo not found")
		return
	}
	if r.Method == http.MethodPost {
		var q struct {
			Text string `json:"text"`
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		q.Text = strings.TrimSpace(q.Text)
		if n := utf8.RuneCountInString(q.Text); n < 1 || n > 2000 {
			writeError(w, 422, "validation_failed", "text must be 1-2000 characters")
			return
		}
		var commentID int64
		err = s.store.Pool().QueryRow(r.Context(), `INSERT INTO photo_comments(photo_id,author_id,text) VALUES($1::uuid,$2,$3) RETURNING id`, id, userIDFrom(r.Context()), q.Text).Scan(&commentID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, 201, map[string]any{"id": commentID})
		return
	}
	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			writeError(w, 400, "bad_request", "invalid cursor")
			return
		}
	}
	items, err := s.store.PhotoComments(r.Context(), id, after)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var next *int64
	if len(items) > 50 {
		items = items[:50]
		n := items[49].ID
		next = &n
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
}
