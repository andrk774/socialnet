package httpapi

import (
	"errors"
	"net/http"
	"socialnet/internal/store"
)

func (s *Server) visibleComment(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return 0, false
	}
	meta, err := s.store.CommentMeta(r.Context(), id)
	if err == nil && meta.ParentID != nil {
		_, err = s.store.CommentMeta(r.Context(), *meta.ParentID)
	}
	if err == nil {
		_, err = s.store.PostForViewer(r.Context(), userIDFrom(r.Context()), meta.PostID)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "not_found", "comment not found")
		return 0, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return 0, false
	}
	return id, true
}
func (s *Server) handleCommentReact(w http.ResponseWriter, r *http.Request) {
	id, ok := s.visibleComment(w, r)
	if !ok {
		return
	}
	var req struct {
		Reaction string `json:"reaction"`
	}
	if r.Method != "DELETE" {
		if !decodeJSON(w, r, &req) {
			return
		}
		valid := false
		for _, x := range store.Reactions {
			if x == req.Reaction {
				valid = true
			}
		}
		if !valid {
			writeError(w, 422, "invalid_reaction", "unknown reaction")
			return
		}
	}
	if err := s.store.CommentReact(r.Context(), id, userIDFrom(r.Context()), req.Reaction); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleCommentReactors(w http.ResponseWriter, r *http.Request) {
	id, ok := s.visibleComment(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	items, err := s.store.CommentReactors(r.Context(), id, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, 200, list(items, p.Limit, func(x store.CommentReactor) int64 { return x.User.ID }))
}
