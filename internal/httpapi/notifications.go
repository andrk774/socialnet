package httpapi

import (
	"net/http"

	"socialnet/internal/store"
)

func notifCursor(n store.Notification) int64 { return n.ID }

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	p := pageParams(r)
	items, err := s.store.Notifications(r.Context(), me, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	unread, err := s.store.UnreadNotifications(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		listResponse[store.Notification]
		Unread int `json:"unread"`
	}{list(items, p.Limit, notifCursor), unread})
}

// handleNotificationsRead: {"ids": [1,2]} или {} — прочитать все.
func (s *Server) handleNotificationsRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) > 500 {
		writeError(w, http.StatusBadRequest, "bad_request", "too many ids")
		return
	}
	if err := s.store.MarkNotificationsRead(r.Context(), userIDFrom(r.Context()), req.IDs); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
