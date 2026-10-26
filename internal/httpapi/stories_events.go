package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"socialnet/internal/store"
)

// ---------- истории ----------

func (s *Server) handleCreateStory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PhotoID *string `json:"photo_id"`
		VideoID *string `json:"video_id"`
		Caption string  `json:"caption"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	norm := func(v *string) *string {
		if v == nil {
			return nil
		}
		x := strings.ToLower(*v)
		if !validUUID(x) {
			errs["media"] = "invalid id"
		}
		return &x
	}
	photo, video := norm(req.PhotoID), norm(req.VideoID)
	if (photo == nil) == (video == nil) {
		errs["media"] = "exactly one of photo_id or video_id is required"
	}
	caption := cleanText(req.Caption, 200)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	id, err := s.store.CreateStory(r.Context(), me, photo, video, caption)
	if errors.Is(err, store.ErrBadMedia) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"media": "photo or video not found, not yours, or video is not ready"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "expires_in_hours": 24})
}

func (s *Server) handleStoriesFeed(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.StoriesFeed(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": groups})
}

func (s *Server) handleUserStories(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	groups, err := s.store.UserStories(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": groups})
}

func (s *Server) handleViewStory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	visible, err := s.store.ViewStory(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !visible {
		writeError(w, http.StatusNotFound, "not_found", "story not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStoryViewers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.store.StoryViewers(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "story not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNil(items)})
}

func (s *Server) handleDeleteStory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := s.store.DeleteStory(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "story not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
