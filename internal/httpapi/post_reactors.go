package httpapi

import (
	"net/http"
	"socialnet/internal/store"
)

func (s *Server) handlePostReactors(w http.ResponseWriter, r *http.Request) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	items, err := s.store.PostReactors(r.Context(), post.ID, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(item store.PostReactor) int64 { return item.User.ID }))
}
