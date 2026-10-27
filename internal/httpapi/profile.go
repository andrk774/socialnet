package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.store.UserByID(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	v := toSelf(user)
	v.ConsentRequired = viewerFrom(r.Context()).NeedsConsent
	if v.Plus, err = s.store.HasPlus(r.Context(), user.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type profileRequest struct {
	FirstName  *string `json:"first_name"`
	LastName   *string `json:"last_name"`
	StatusText *string `json:"status_text"`
	City       *string `json:"city"`
	About      *string `json:"about"`
}

// trimField обрезает пробелы и проверяет длину в символах (не байтах).
func trimField(errs map[string]string, name string, v *string, max int) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if utf8.RuneCountInString(t) > max {
		errs[name] = "too long"
	}
	return &t
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	upd := store.ProfileUpdate{
		FirstName:  trimField(errs, "first_name", req.FirstName, 64),
		LastName:   trimField(errs, "last_name", req.LastName, 64),
		StatusText: trimField(errs, "status_text", req.StatusText, 140),
		City:       trimField(errs, "city", req.City, 64),
		About:      trimField(errs, "about", req.About, 1000),
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	user, err := s.store.UpdateProfile(r.Context(), userIDFrom(r.Context()), upd)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSelf(user))
}

type privacyRequest struct {
	OnlineVisibility  *string `json:"online_visibility"`
	ProfileVisibility *string `json:"profile_visibility"`
	FriendsVisibility *string `json:"friends_visibility"`
	MessagePrivacy    *string `json:"message_privacy"`
	Searchable        *bool   `json:"searchable"`
}

func oneOf(errs map[string]string, name string, v *string, allowed ...string) {
	if v == nil {
		return
	}
	for _, a := range allowed {
		if *v == a {
			return
		}
	}
	errs[name] = "must be one of: " + strings.Join(allowed, ", ")
}

func (s *Server) handleUpdatePrivacy(w http.ResponseWriter, r *http.Request) {
	var req privacyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	oneOf(errs, "online_visibility", req.OnlineVisibility, "everyone", "friends", "only_me")
	oneOf(errs, "profile_visibility", req.ProfileVisibility, "everyone", "friends", "only_me")
	oneOf(errs, "friends_visibility", req.FriendsVisibility, "everyone", "friends", "only_me")
	oneOf(errs, "message_privacy", req.MessagePrivacy, "everyone", "friends", "nobody")
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	user, err := s.store.UpdatePrivacy(r.Context(), userIDFrom(r.Context()), store.PrivacyUpdate{
		OnlineVisibility:  req.OnlineVisibility,
		ProfileVisibility: req.ProfileVisibility, FriendsVisibility: req.FriendsVisibility,
		MessagePrivacy: req.MessagePrivacy, Searchable: req.Searchable,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSelf(user))
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := s.store.UserByID(r.Context(), id)
	s.respondProfile(w, r, user, err)
}

func (s *Server) handleGetUserByUsername(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "username")
	if !usernameRe.MatchString(name) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	user, err := s.store.UserByUsername(r.Context(), name)
	s.respondProfile(w, r, user, err)
}

func (s *Server) respondProfile(w http.ResponseWriter, r *http.Request, user *store.User, err error) {
	if errors.Is(err, store.ErrNotFound) || (err == nil && user.BannedAt != nil) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	me := userIDFrom(r.Context())
	rel, err := s.store.Relationship(r.Context(), me, user.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	v := toProfile(user, me == user.ID, rel)
	v.Presence = s.presence(user.ID, user.LastSeenAt, user.OnlineVisibility, me == user.ID, rel)
	if v.Plus, err = s.store.HasPlus(r.Context(), user.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// targetUser загружает пользователя из {id} и связь с ним.
// Забаненный и несуществующий одинаково дают 404.
func (s *Server) targetUser(w http.ResponseWriter, r *http.Request) (*store.User, store.Relationship, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, store.Relationship{}, false
	}
	user, err := s.store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && user.BannedAt != nil) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return nil, store.Relationship{}, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, store.Relationship{}, false
	}
	rel, err := s.store.Relationship(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		s.internalError(w, r, err)
		return nil, store.Relationship{}, false
	}
	return user, rel, true
}
