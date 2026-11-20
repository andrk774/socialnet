package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/mail"
	"socialnet/internal/auth"
	"socialnet/internal/store"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Server) handleAdminProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var p store.AdminProfile
	if !decodeJSON(w, r, &p) {
		return
	}
	p.Email = normalizeEmail(p.Email)
	p.FirstName = strings.TrimSpace(p.FirstName)
	p.LastName = strings.TrimSpace(p.LastName)
	errs := map[string]string{}
	if a, e := mail.ParseAddress(p.Email); e != nil || a.Address != p.Email || len(p.Email) > 254 {
		errs["email"] = "invalid email"
	}
	if utf8.RuneCountInString(p.FirstName) > 64 || p.FirstName == "" {
		errs["first_name"] = "1-64 characters"
	}
	if utf8.RuneCountInString(p.LastName) > 64 {
		errs["last_name"] = "max 64 characters"
	}
	now := time.Now()
	if p.CreatedAt.IsZero() || p.CreatedAt.Before(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) || p.CreatedAt.After(now) {
		errs["created_at"] = "date must be between 1970 and now"
	}
	if p.Birthday != nil {
		d, e := time.Parse("2006-01-02", *p.Birthday)
		if e != nil || d.After(now) || d.Before(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
			errs["birthday"] = "invalid birthday"
		}
	}
	if len(errs) > 0 {
		writeFieldErrors(w, 422, "validation_failed", "invalid input", errs)
		return
	}
	err := s.store.AdminUpdateProfile(r.Context(), id, p)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		writeFieldErrors(w, 409, "conflict", "email already used", map[string]string{"email": "already used"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "not_found", "user not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "edit_profile", "user", id, nil, "")
	w.WriteHeader(204)
}
func (s *Server) handleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validPassword(req.Password) {
		writeFieldErrors(w, 422, "validation_failed", "invalid password", map[string]string{"password": "8-128 characters"})
		return
	}
	if _, err := s.store.UserByID(r.Context(), id); err != nil {
		s.moneyError(w, r, err)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err == nil {
		err = s.store.AdminSetPassword(r.Context(), id, hash)
	}
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	_ = s.store.AddSecurityEvent(r.Context(), id, "admin_password_changed", store.SessionMeta{})
	s.modLog(r, "set_password", "user", id, nil, "sessions revoked")
	w.WriteHeader(204)
}
