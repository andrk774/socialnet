package httpapi

import (
	"errors"
	"net/http"
	"net/url"

	"socialnet/internal/auth"
	"socialnet/internal/store"
)

// Раздел администратора. Пароли здесь не видны ни в каком виде: в базе
// лежит только необратимый хэш Argon2id. Помочь пользователю, который не
// может войти, админ может письмом со ссылкой на сброс — ссылку получает
// только владелец почты, админ её не видит.

// handleAdminStats: GET /admin/stats — сводка для главной страницы админки.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.AdminStats(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleAdminUsers: GET /admin/users?q=&filter=all|banned|staff|unverified&before=
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := q.Get("filter")
	if filter == "" {
		filter = "all"
	}
	errs := map[string]string{}
	oneOf(errs, "filter", &filter, "all", "banned", "staff", "unverified")
	if len([]rune(q.Get("q"))) > 100 {
		errs["q"] = "max 100 characters"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	p := pageParams(r)
	items, err := s.store.AdminSearchUsers(r.Context(), q.Get("q"), filter, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(u store.AdminUser) int64 { return u.ID }))
}

// handleAdminUser: GET /admin/users/{id} — карточка пользователя.
func (s *Server) handleAdminUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	card, err := s.store.AdminUserCard(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// handleAdminPasswordReset: POST /admin/users/{id}/password-reset {"revoke_sessions": bool}
// Отправляет пользователю письмо со ссылкой на сброс. Пароль не меняется,
// пока владелец почты сам не перейдёт по ссылке.
func (s *Server) handleAdminPasswordReset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		RevokeSessions bool `json:"revoke_sessions"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if bot, err := s.store.BotCommunity(r.Context(), user.ID); err != nil {
		s.internalError(w, r, err)
		return
	} else if bot != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "bots have no password")
		return
	}
	if user.BannedAt != nil {
		writeError(w, http.StatusConflict, "banned", "unban the user first")
		return
	}
	token, hash, err := auth.NewEmailToken()
	if err == nil {
		err = s.store.CreateAuthToken(r.Context(), user.ID, "reset_password", hash, resetPasswordTTL)
	}
	if err == nil && req.RevokeSessions {
		err = s.store.RevokeUserEverywhere(r.Context(), user.ID)
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.sendMail(user.Email, "Сброс пароля",
		"Администратор сайта отправил вам ссылку для смены пароля — обычно это делают по вашей просьбе в поддержку.\n\n"+
			"Ссылка действует 1 час:\n"+s.cfg.AppURL+"/reset-password?token="+url.QueryEscape(token)+"\n\n"+
			"Если вы ни о чём не просили, просто ничего не делайте: пароль останется прежним.\n")
	// В журнал пользователя — без IP и браузера админа.
	s.m.events.Inc("admin_password_reset")
	if err := s.store.AddSecurityEvent(r.Context(), user.ID, "admin_password_reset", store.SessionMeta{}); err != nil {
		s.log.Error("security event", "err", err)
	}
	note := ""
	if req.RevokeSessions {
		note = "sessions revoked"
	}
	s.modLog(r, "password_reset_email", "user", user.ID, nil, note)
	writeJSON(w, http.StatusAccepted, map[string]any{"sent_to": maskEmail(user.Email), "sessions_revoked": req.RevokeSessions})
}

// handleAdminRevokeSessions: POST /admin/users/{id}/revoke-sessions — выйти отовсюду
// (сеансы и токены приложений). Пригодится, если аккаунт угнан.
func (s *Server) handleAdminRevokeSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if id == userIDFrom(r.Context()) {
		writeError(w, http.StatusBadRequest, "bad_request", "use your own sessions page")
		return
	}
	if _, err := s.store.UserByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.RevokeUserEverywhere(r.Context(), id); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.modLog(r, "revoke_sessions", "user", id, nil, "")
	w.WriteHeader(http.StatusNoContent)
}

// maskEmail: a***@example.com — админу достаточно понять, куда ушло письмо.
func maskEmail(e string) string {
	for i := 0; i < len(e); i++ {
		if e[i] == '@' {
			if i <= 1 {
				return "*" + e[i:]
			}
			return e[:1] + "***" + e[i:]
		}
	}
	return "***"
}
