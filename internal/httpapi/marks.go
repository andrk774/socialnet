package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"socialnet/internal/gpx"
	"socialnet/internal/store"
)

// Точки на маршруте: отмечает любой, кто видит маршрут (ходил по нему),
// удаляет автор точки или владелец маршрута. Точка должна быть рядом с
// треком — не дальше maxMarkDistance.

const maxMarkDistance = 2000.0 // м

var markKinds = []string{"spring", "camp", "ford", "danger", "view", "other"}

// distanceToTrack — до ближайшей точки трека (точности вершин хватает:
// упрощённый трек не реже чем раз в несколько сотен метров).
func distanceToTrack(route *store.Route, lat, lon float64) float64 {
	best := math.Inf(1)
	for _, seg := range route.Detail {
		for _, p := range seg {
			if d := gpx.Distance(p[0], p[1], lat, lon); d < best {
				best = d
			}
		}
	}
	return best
}

func (s *Server) handleAddMark(w http.ResponseWriter, r *http.Request) {
	route, ok := s.visibleRoute(w, r, true)
	if !ok {
		return
	}
	var req struct {
		Kind    string  `json:"kind"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Title   string  `json:"title"`
		Note    string  `json:"note"`
		PhotoID *string `json:"photo_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "kind", &req.Kind, markKinds...)
	title := cleanText(req.Title, 80)
	note := strings.TrimSpace(req.Note)
	if title == "" {
		errs["title"] = "1-80 characters"
	}
	if utf8.RuneCountInString(note) > 1000 {
		errs["note"] = "max 1000 characters"
	}
	if math.IsNaN(req.Lat) || math.IsNaN(req.Lon) || req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		errs["lat"] = "invalid coordinates"
	} else if distanceToTrack(route, req.Lat, req.Lon) > maxMarkDistance {
		errs["lat"] = "the point must be within 2 km of the track"
	}
	var photo *string
	if req.PhotoID != nil {
		id := strings.ToLower(*req.PhotoID)
		if !validUUID(id) {
			errs["photo_id"] = "photo not found"
		}
		photo = &id
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	id, err := s.store.AddRoutePoint(r.Context(), store.NewRoutePoint{RouteID: route.ID, AuthorID: me, Kind: req.Kind,
		Lat: round6(req.Lat), Lon: round6(req.Lon), Title: title, Note: note, PhotoID: photo})
	switch {
	case errors.Is(err, store.ErrBadPhoto):
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"photo_id": "photo not found"})
		return
	case errors.Is(err, store.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many points today")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	p, err := s.store.RoutePoint(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// markOfRoute — точка этого (видимого) маршрута.
func (s *Server) markOfRoute(w http.ResponseWriter, r *http.Request) (*store.RoutePoint, bool) {
	route, ok := s.visibleRoute(w, r, false)
	if !ok {
		return nil, false
	}
	pid, ok := pathID(w, r, "pid")
	if !ok {
		return nil, false
	}
	p, err := s.store.RoutePoint(r.Context(), userIDFrom(r.Context()), pid)
	if errors.Is(err, store.ErrNotFound) || err == nil && p.RouteID != route.ID {
		writeError(w, http.StatusNotFound, "not_found", "point not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return p, true
}

// handleCheckMark: POST /routes/{rid}/marks/{pid}/check — «на месте, актуально».
func (s *Server) handleCheckMark(w http.ResponseWriter, r *http.Request) {
	p, ok := s.markOfRoute(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if err := s.store.CheckRoutePoint(r.Context(), p.ID, me); err != nil {
		s.internalError(w, r, err)
		return
	}
	p, err := s.store.RoutePoint(r.Context(), me, p.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteMark(w http.ResponseWriter, r *http.Request) {
	p, ok := s.markOfRoute(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteRoutePoint(r.Context(), p.ID, userIDFrom(r.Context())); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusForbidden, "forbidden", "only the author or the route owner can delete")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
