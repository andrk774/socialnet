package httpapi

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleOwnComments(w http.ResponseWriter, r *http.Request) {
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		var err error
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			writeError(w, 400, "bad_request", "invalid cursor")
			return
		}
	}
	items, err := s.store.OwnComments(r.Context(), userIDFrom(r.Context()), after)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var next *int64
	if len(items) == 200 {
		last := items[len(items)-1].ID
		next = &last
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
}

func (s *Server) handlePersonalStats(w http.ResponseWriter, r *http.Request) {
	start, e1 := time.Parse("2006-01-02", r.URL.Query().Get("start"))
	last, e2 := time.Parse("2006-01-02", r.URL.Query().Get("end"))
	if e1 != nil || e2 != nil || last.Before(start) || last.Sub(start) > 366*24*time.Hour {
		writeError(w, 400, "bad_request", "valid start and end dates required; maximum 367 days")
		return
	}
	items, err := s.store.PersonalStats(r.Context(), userIDFrom(r.Context()), start, last.AddDate(0, 0, 1))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	truncated := len(items) > 1000
	if truncated {
		items = items[:1000]
	}
	writeJSON(w, 200, map[string]any{"items": items, "truncated": truncated})
}
