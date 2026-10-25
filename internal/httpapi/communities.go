package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
)

func (s *Server) handleCreateCommunity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"` // open (по умолчанию) | closed
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Slug, req.Name, req.Description = strings.TrimSpace(req.Slug), strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	errs := map[string]string{}
	if !usernameRe.MatchString(req.Slug) {
		errs["slug"] = "3-32 characters: latin letters, digits, underscore"
	}
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > 100 {
		errs["name"] = "1-100 characters"
	}
	if utf8.RuneCountInString(req.Description) > 2000 {
		errs["description"] = "max 2000 characters"
	}
	if req.Visibility == "" {
		req.Visibility = "open"
	}
	oneOf(errs, "visibility", &req.Visibility, "open", "closed")
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	id, err := s.store.CreateCommunity(r.Context(), me, req.Slug, req.Name, req.Description, req.Visibility)
	if errors.Is(err, store.ErrSlugTaken) {
		writeFieldErrors(w, http.StatusConflict, "conflict", "already taken", map[string]string{"slug": "slug already taken"})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	c, err := s.store.CommunityByID(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// community загружает сообщество {id}. Скрытое модератором — 404 для всех,
// кроме модераторов.
func (s *Server) community(w http.ResponseWriter, r *http.Request) (*store.Community, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	v := viewerFrom(r.Context())
	c, err := s.store.CommunityByID(r.Context(), v.ID, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && c.Hidden && v.Role == "user") {
		writeError(w, http.StatusNotFound, "not_found", "community not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return c, true
}

func (s *Server) handleGetCommunity(w http.ResponseWriter, r *http.Request) {
	if c, ok := s.community(w, r); ok {
		writeJSON(w, http.StatusOK, c)
	}
}

func (s *Server) handleGetCommunityBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	v := viewerFrom(r.Context())
	if !usernameRe.MatchString(slug) {
		writeError(w, http.StatusNotFound, "not_found", "community not found")
		return
	}
	c, err := s.store.CommunityBySlug(r.Context(), v.ID, slug)
	if errors.Is(err, store.ErrNotFound) || (err == nil && c.Hidden && v.Role == "user") {
		writeError(w, http.StatusNotFound, "not_found", "community not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleUpdateCommunity(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Visibility  *string `json:"visibility"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !store.RoleAtLeast(c.MyRole, "admin") {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can edit the community")
		return
	}
	errs := map[string]string{}
	name := trimField(errs, "name", req.Name, 100)
	desc := trimField(errs, "description", req.Description, 2000)
	if name != nil && *name == "" {
		errs["name"] = "1-100 characters"
	}
	oneOf(errs, "visibility", req.Visibility, "open", "closed")
	if req.Visibility != nil && c.MyRole != "owner" {
		errs["visibility"] = "only the owner can change visibility"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.UpdateCommunity(r.Context(), c.ID, name, desc); err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.Visibility != nil {
		if err := s.store.SetCommunityVisibility(r.Context(), c.ID, *req.Visibility); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	updated, err := s.store.CommunityByID(r.Context(), userIDFrom(r.Context()), c.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleJoinCommunity(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if c.Visibility == "closed" && c.MyRole == "" {
		// в закрытое — по заявке, её одобряют админы
		created, err := s.store.RequestJoin(r.Context(), c.ID, me)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if created {
			s.notify(r.Context(), store.NewNotification{UserID: c.OwnerID, Type: "join_request", ActorID: me})
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "requested"})
		return
	}
	if err := s.store.JoinCommunity(r.Context(), c.ID, me); err != nil {
		s.internalError(w, r, err)
		return
	}
	if c.MyRole == "" {
		s.emit(r.Context(), c.ID, "group_join", map[string]int64{"user_id": me})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLeaveCommunity(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	if c.MyRole == "owner" {
		writeError(w, http.StatusConflict, "owner_cannot_leave", "owner cannot leave the community")
		return
	}
	if err := s.store.RemoveMember(r.Context(), c.ID, userIDFrom(r.Context())); err != nil {
		s.internalError(w, r, err)
		return
	}
	if c.MyRole != "" {
		s.emit(r.Context(), c.ID, "group_leave", map[string]int64{"user_id": userIDFrom(r.Context())})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCommunityMembers(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	if c.Visibility == "closed" && c.MyRole == "" {
		writeError(w, http.StatusForbidden, "private", "members of a closed community are visible to members only")
		return
	}
	p := pageParams(r)
	items, err := s.store.Members(r.Context(), c.ID, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(m store.Member) int64 { return m.ID }))
}

// canManageMember: кто может менять роль или исключать участника.
//   - владельца не трогает никто;
//   - админа может понизить или исключить только владелец;
//   - назначить админа может только владелец;
//   - админ управляет редакторами и участниками.
func canManageMember(actor, target, newRole string) bool {
	if target == "owner" || target == "" {
		return false
	}
	if actor == "owner" {
		return true
	}
	if actor == "admin" {
		return target != "admin" && newRole != "admin"
	}
	return false
}

func (s *Server) memberTarget(w http.ResponseWriter, r *http.Request) (*store.Community, int64, string, bool) {
	c, ok := s.community(w, r)
	if !ok {
		return nil, 0, "", false
	}
	uid, ok := pathID(w, r, "userID")
	if !ok {
		return nil, 0, "", false
	}
	role, err := s.store.CommunityRole(r.Context(), c.ID, uid)
	if err != nil {
		s.internalError(w, r, err)
		return nil, 0, "", false
	}
	if role == "" {
		writeError(w, http.StatusNotFound, "not_found", "member not found")
		return nil, 0, "", false
	}
	return c, uid, role, true
}

func (s *Server) handleSetMemberRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Role != "admin" && req.Role != "editor" && req.Role != "member" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"role": "must be one of: admin, editor, member"})
		return
	}
	c, uid, role, ok := s.memberTarget(w, r)
	if !ok {
		return
	}
	if !canManageMember(c.MyRole, role, req.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "not allowed")
		return
	}
	if err := s.store.SetMemberRole(r.Context(), c.ID, uid, req.Role); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": uid, "role": req.Role})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	c, uid, role, ok := s.memberTarget(w, r)
	if !ok {
		return
	}
	if !canManageMember(c.MyRole, role, "") {
		writeError(w, http.StatusForbidden, "forbidden", "not allowed")
		return
	}
	if err := s.store.RemoveMember(r.Context(), c.ID, uid); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCommunityWall(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	p := pageParams(r)
	posts, err := s.store.CommunityWall(r.Context(), userIDFrom(r.Context()), c.ID, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(posts, p.Limit, postCursor))
}

func (s *Server) handleMyCommunities(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.MyCommunities(r.Context(), userIDFrom(r.Context()), p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(c store.Community) int64 { return c.ID }))
}

// ---------- поиск ----------

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(q); n < 2 || n > 64 {
		writeError(w, http.StatusBadRequest, "bad_request", "query must be 2-64 characters")
		return
	}
	limit := pageParams(r).Limit
	me := userIDFrom(r.Context())
	switch r.URL.Query().Get("type") {
	case "", "users":
		items, err := s.store.SearchUsers(r.Context(), me, q, limit)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": nonNil(items)})
	case "tracks":
		items, err := s.store.SearchTracks(r.Context(), me, q, limit)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case "communities":
		items, err := s.store.SearchCommunities(r.Context(), me, q, limit)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "type must be users, communities or tracks")
	}
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// ---------- заявки в закрытое сообщество ----------

func (s *Server) handleJoinRequests(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	if !store.RoleAtLeast(c.MyRole, "admin") {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can see join requests")
		return
	}
	p := pageParams(r)
	items, err := s.store.JoinRequests(r.Context(), c.ID, p.After, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, cardCursor))
}

func (s *Server) handleResolveJoin(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := s.community(w, r)
		if !ok {
			return
		}
		uid, ok := pathID(w, r, "userID")
		if !ok {
			return
		}
		if !store.RoleAtLeast(c.MyRole, "admin") {
			writeError(w, http.StatusForbidden, "forbidden", "only admins can resolve join requests")
			return
		}
		err := s.store.ResolveJoinRequest(r.Context(), c.ID, uid, approve)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "request not found")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if approve {
			s.emit(r.Context(), c.ID, "group_join", map[string]int64{"user_id": uid})
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
