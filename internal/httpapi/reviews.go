package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

type reviewRequest struct {
	Rating int    `json:"rating"`
	Text   string `json:"text"`
}

func decodeReview(w http.ResponseWriter, r *http.Request) (*reviewRequest, bool) {
	var req reviewRequest
	if !decodeJSON(w, r, &req) {
		return nil, false
	}
	req.Text = strings.TrimSpace(req.Text)
	errs := map[string]string{}
	if req.Rating < 1 || req.Rating > 5 {
		errs["rating"] = "1-5"
	}
	if utf8.RuneCountInString(req.Text) > 2000 {
		errs["text"] = "up to 2000 characters"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return nil, false
	}
	return &req, true
}

func (s *Server) reviewError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, store.ErrCannotReview):
		writeError(w, http.StatusForbidden, "not_participant", "only participants can leave a review, and not about themselves")
	case errors.Is(err, store.ErrReviewTooEarly):
		writeError(w, http.StatusConflict, "too_early", "the hike has not ended yet")
	case errors.Is(err, store.ErrReviewTooLate):
		writeError(w, http.StatusConflict, "too_late", "reviews are accepted for 90 days after the hike")
	default:
		s.internalError(w, r, err)
	}
}

// handleReviewEvent: POST /events/{id}/review — участник об организаторе.
func (s *Server) handleReviewEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	req, ok := decodeReview(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	existed, err := s.store.MyEventReview(r.Context(), e.ID, me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	id, err := s.store.ReviewOrganizer(r.Context(), e.ID, me, req.Rating, req.Text)
	if err != nil {
		s.reviewError(w, r, err)
		return
	}
	if existed == nil {
		s.notifyEvent(r.Context(), []int64{e.CreatorID}, "hike_review", me, e.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) handleDeleteEventReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteMyReview(r.Context(), userIDFrom(r.Context()), &id, nil); err != nil {
		s.reviewError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUserReviews: GET /users/{id}/reviews — отзывы об организаторе.
func (s *Server) handleUserReviews(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p := pageParams(r)
	rating, items, err := s.store.OrganizerReviews(r.Context(), id, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := list(items, p.Limit, func(rv store.Review) int64 { return rv.ID })
	writeJSON(w, http.StatusOK, map[string]any{"rating": rating, "items": out.Items, "next_cursor": out.NextCursor})
}

// routeForReviews — маршрут каталога или доступный пользователю.
func (s *Server) routeForReviews(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(chi.URLParam(r, "rid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return "", false
	}
	ok, err := s.store.RouteAccess(r.Context(), userIDFrom(r.Context()), id)
	if err == nil && !ok {
		_, perr := s.store.PublicRouteByID(r.Context(), id)
		ok = perr == nil
	}
	if err != nil {
		s.internalError(w, r, err)
		return "", false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return "", false
	}
	return id, true
}

func (s *Server) handleRouteReviews(w http.ResponseWriter, r *http.Request) {
	id, ok := s.routeForReviews(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	rating, items, err := s.store.RouteReviews(r.Context(), id, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := list(items, p.Limit, func(rv store.Review) int64 { return rv.ID })
	writeJSON(w, http.StatusOK, map[string]any{"rating": rating, "items": out.Items, "next_cursor": out.NextCursor})
}

// handleReviewRoute: POST /routes/{id}/reviews — о маршруте каталога.
func (s *Server) handleReviewRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := s.routeForReviews(w, r)
	if !ok {
		return
	}
	req, ok := decodeReview(w, r)
	if !ok {
		return
	}
	rid, err := s.store.ReviewRoute(r.Context(), id, userIDFrom(r.Context()), req.Rating, req.Text)
	if err != nil {
		s.reviewError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": rid})
}

func (s *Server) handleDeleteRouteReview(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(chi.URLParam(r, "rid"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err := s.store.DeleteMyReview(r.Context(), userIDFrom(r.Context()), nil, &id); err != nil {
		s.reviewError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// reviewState — может ли участник оценить поход и что он уже написал.
func (s *Server) reviewState(r *http.Request, e *store.Event) {
	me := userIDFrom(r.Context())
	end := e.StartsAt.Add(6 * time.Hour)
	if e.EndsAt != nil {
		end = *e.EndsAt
	}
	if e.Kind != "hike" || e.CreatorID == me || e.MyStatus == nil || *e.MyStatus != "going" ||
		time.Now().Before(end) || time.Since(end) > store.ReviewWindow {
		return
	}
	e.CanReview = true
	e.MyReview, _ = s.store.MyEventReview(r.Context(), e.ID, me)
}
