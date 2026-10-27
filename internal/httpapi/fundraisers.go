package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/store"
)

// Сборы сообществ: цель, срок, прогресс. Объявляют администраторы,
// пожертвования — монетами на счёт сообщества (подтверждённая почта,
// ключ идемпотентности, можно анонимно). Выводом денег сбор не занимается:
// это обычный кошелёк сообщества.

func (s *Server) handleCreateFundraiser(w http.ResponseWriter, r *http.Request) {
	c, ok := s.communityAdmin(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Goal        int64      `json:"goal"`
		Deadline    *time.Time `json:"deadline"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	title := cleanText(req.Title, 120)
	desc := strings.TrimSpace(req.Description)
	if title == "" {
		errs["title"] = "1-120 characters"
	}
	if utf8.RuneCountInString(desc) > 2000 {
		errs["description"] = "max 2000 characters"
	}
	if req.Goal < 1 || req.Goal > 10_000_000 {
		errs["goal"] = "1-10000000 coins"
	}
	if req.Deadline == nil || req.Deadline.Before(time.Now().Add(time.Hour)) || req.Deadline.After(time.Now().Add(180*24*time.Hour)) {
		errs["deadline"] = "between 1 hour and 180 days from now"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	id, err := s.store.CreateFundraiser(r.Context(), c.ID, me, title, desc, req.Goal, *req.Deadline)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	f, err := s.store.Fundraiser(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleCommunityFundraisers(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	items, err := s.store.Fundraisers(r.Context(), userIDFrom(r.Context()), c.ID, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFundraisers(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Fundraisers(r.Context(), userIDFrom(r.Context()), 0, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) fundraiserParam(w http.ResponseWriter, r *http.Request) (*store.Fundraiser, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	f, err := s.store.Fundraiser(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "fundraiser not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return f, true
}

func (s *Server) handleGetFundraiser(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fundraiserParam(w, r)
	if !ok {
		return
	}
	donations, err := s.store.Donations(r.Context(), f.ID, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fundraiser": f, "donations": donations})
}

// handleDonateFundraiser: POST /fundraisers/{id}/donate.
func (s *Server) handleDonateFundraiser(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fundraiserParam(w, r)
	if !ok {
		return
	}
	var req struct {
		moneyRequest
		Anonymous bool `json:"anonymous"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(r); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	rec, err := s.store.Donate(r.Context(), f.ID, me.ID, req.Amount, req.Anonymous, cleanText(req.Memo, 200), req.IdempotencyKey)
	if errors.Is(err, store.ErrFundraiserClosed) {
		writeError(w, http.StatusConflict, "fundraiser_closed", "the fundraiser is over")
		return
	}
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	if !rec.Replayed {
		actor := me.ID
		if req.Anonymous {
			actor = 0
		}
		if owner, err := s.store.CommunityByID(r.Context(), me.ID, f.Community.ID); err == nil {
			s.notify(r.Context(), store.NewNotification{UserID: owner.OwnerID, Type: "donation", ActorID: actor})
		}
		s.m.business.Inc("fundraiser_donation")
	}
	s.respondTx(w, r, rec, store.UserAccount(me.ID))
}

func (s *Server) handleCloseFundraiser(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fundraiserParam(w, r)
	if !ok {
		return
	}
	role, err := s.store.CommunityRole(r.Context(), f.Community.ID, userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !store.RoleAtLeast(role, "admin") {
		writeError(w, http.StatusForbidden, "forbidden", "only community admins can close a fundraiser")
		return
	}
	if err := s.store.CloseFundraiser(r.Context(), f.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	f, _ = s.fundraiserParam(w, r)
	writeJSON(w, http.StatusOK, f)
}
