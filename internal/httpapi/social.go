package httpapi

import (
	"net/http"

	"socialnet/internal/store"
)

// handleFriendAdd: отправить заявку или принять входящую (взаимная подписка).
func (s *Server) handleFriendAdd(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if target.ID == me {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot add yourself")
		return
	}
	if rel.Blocked() {
		writeError(w, http.StatusForbidden, "blocked", "action unavailable")
		return
	}
	created, err := s.store.Follow(r.Context(), me, target.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rel.IFollow = true
	if created {
		typ := "friend_request"
		if rel.FollowsMe {
			typ = "friend_accepted"
		}
		s.notify(r.Context(), store.NewNotification{UserID: target.ID, Type: typ, ActorID: me})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": relStatus(false, rel)})
}

// handleFriendRemove: отменить заявку или удалить из друзей.
// Как во ВКонтакте, бывший друг остаётся подписчиком.
func (s *Server) handleFriendRemove(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if err := s.store.Unfollow(r.Context(), userIDFrom(r.Context()), target.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	rel.IFollow = false
	writeJSON(w, http.StatusOK, map[string]string{"status": relStatus(false, rel)})
}

// handleFriendReject: отклонить входящую заявку.
func (s *Server) handleFriendReject(w http.ResponseWriter, r *http.Request) {
	target, _, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if err := s.store.RejectFollower(r.Context(), userIDFrom(r.Context()), target.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func cardCursor(c store.UserCard) int64 { return c.ID }

func (s *Server) handleFriendsList(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	self := target.ID == userIDFrom(r.Context())
	if !canSee(target.FriendsVisibility, self, rel) {
		writeError(w, http.StatusForbidden, "private", "friends list is hidden")
		return
	}
	p := pageParams(r)
	items, err := s.store.Friends(r.Context(), target.ID, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, cardCursor))
}

func (s *Server) handleFriendRequests(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	me := userIDFrom(r.Context())
	var (
		items []store.UserCard
		err   error
	)
	switch r.URL.Query().Get("direction") {
	case "", "incoming":
		items, err = s.store.IncomingRequests(r.Context(), me, p.After, p.Limit)
	case "outgoing":
		items, err = s.store.OutgoingRequests(r.Context(), me, p.After, p.Limit)
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "direction must be incoming or outgoing")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, cardCursor))
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	target, _, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if target.ID == me {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot block yourself")
		return
	}
	if err := s.store.Block(r.Context(), me, target.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.Unblock(r.Context(), userIDFrom(r.Context()), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBlockedList(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.BlockedUsers(r.Context(), userIDFrom(r.Context()), p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, cardCursor))
}
