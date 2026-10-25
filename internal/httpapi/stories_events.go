package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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

// ---------- мероприятия ----------

type eventRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Place       *string    `json:"place"`
	StartsAt    *time.Time `json:"starts_at"` // RFC 3339, с часовым поясом
	EndsAt      *time.Time `json:"ends_at"`
	CommunityID *int64     `json:"community_id"`
}

func (req *eventRequest) validate(creating bool) map[string]string {
	errs := map[string]string{}
	if req.Title != nil {
		t := cleanText(*req.Title, 200)
		req.Title = &t
		if t == "" {
			errs["title"] = "1-200 characters"
		}
	} else if creating {
		errs["title"] = "required"
	}
	if req.Place != nil {
		p := cleanText(*req.Place, 200)
		req.Place = &p
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(d) > 5000 {
			errs["description"] = "max 5000 characters"
		}
		req.Description = &d
	}
	if creating && req.StartsAt == nil {
		errs["starts_at"] = "required (RFC 3339)"
	}
	if req.StartsAt != nil && req.StartsAt.Before(time.Now().Add(-time.Hour)) && creating {
		errs["starts_at"] = "must be in the future"
	}
	if req.StartsAt != nil && req.EndsAt != nil && !req.EndsAt.After(*req.StartsAt) {
		errs["ends_at"] = "must be after starts_at"
	}
	return errs
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(true); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	if req.CommunityID != nil {
		role, err := s.store.CommunityRole(r.Context(), *req.CommunityID, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if !store.RoleAtLeast(role, "editor") {
			writeError(w, http.StatusForbidden, "forbidden", "only editors can create community events")
			return
		}
	}
	ne := store.NewEvent{CreatorID: me, CommunityID: req.CommunityID, Title: *req.Title,
		StartsAt: *req.StartsAt, EndsAt: req.EndsAt}
	if req.Description != nil {
		ne.Description = *req.Description
	}
	if req.Place != nil {
		ne.Place = *req.Place
	}
	id, err := s.store.CreateEvent(r.Context(), ne)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	e, err := s.store.Event(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) eventParam(w http.ResponseWriter, r *http.Request) (*store.Event, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	e, err := s.store.Event(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return e, true
}

// canManageEvent: организатор или админ сообщества.
func (s *Server) canManageEvent(r *http.Request, e *store.Event) (bool, error) {
	me := userIDFrom(r.Context())
	if e.CreatorID == me {
		return true, nil
	}
	if e.Community != nil {
		role, err := s.store.CommunityRole(r.Context(), e.Community.ID, me)
		return store.RoleAtLeast(role, "admin"), err
	}
	return false, nil
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	if e, ok := s.eventParam(w, r); ok {
		writeJSON(w, http.StatusOK, e)
	}
}

func (s *Server) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	var req eventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CommunityID != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "community cannot be changed")
		return
	}
	if req.StartsAt == nil {
		req.StartsAt = &e.StartsAt
	}
	if errs := req.validate(false); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	allowed, err := s.canManageEvent(r, e)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "not allowed")
		return
	}
	var ends **time.Time
	if req.EndsAt != nil {
		ends = &req.EndsAt
	}
	if err := s.store.UpdateEvent(r.Context(), e.ID, req.Title, req.Description, req.Place, req.StartsAt, ends); err != nil {
		s.internalError(w, r, err)
		return
	}
	updated, err := s.store.Event(r.Context(), userIDFrom(r.Context()), e.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	allowed, err := s.canManageEvent(r, e)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "not allowed")
		return
	}
	if err := s.store.DeleteEvent(r.Context(), e.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRSVP: POST /events/{id}/rsvp {"status": "going" | "maybe"}.
func (s *Server) handleRSVP(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "status", &req.Status, "going", "maybe")
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.RSVP(r.Context(), e.ID, userIDFrom(r.Context()), req.Status); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCancelRSVP(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	if err := s.store.CancelRSVP(r.Context(), e.ID, userIDFrom(r.Context())); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAttendees(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "going"
	}
	if status != "going" && status != "maybe" {
		writeError(w, http.StatusBadRequest, "bad_request", "status must be going or maybe")
		return
	}
	p := pageParams(r)
	items, err := s.store.Attendees(r.Context(), e.ID, status, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, cardCursor))
}

func (s *Server) handleUpcomingEvents(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.UpcomingEvents(r.Context(), userIDFrom(r.Context()), 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCommunityEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	items, err := s.store.CommunityEvents(r.Context(), userIDFrom(r.Context()), c.ID, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
