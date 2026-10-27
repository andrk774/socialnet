package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/auth"
	"socialnet/internal/store"
)

const refreshCookie = "refresh_token"

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

// botNameRe — имена bot<id> зарезервированы за ботами сообществ.
var botNameRe = regexp.MustCompile(`(?i)^bot[0-9]+$`)

type registerRequest struct {
	Email     string `json:"email"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	consentRequest
	CaptchaToken string `json:"captcha_token"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        *selfView `json:"user,omitempty"`
}

func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func validPassword(p string) bool {
	n := utf8.RuneCountInString(p)
	return n >= 8 && n <= 128
}

func (req *registerRequest) validate() map[string]string {
	errs := map[string]string{}
	req.consentRequest.validate(errs)

	req.Email = normalizeEmail(req.Email)
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email || len(req.Email) > 254 {
		errs["email"] = "invalid email address"
	}

	req.Username = strings.TrimSpace(req.Username)
	if !usernameRe.MatchString(req.Username) {
		errs["username"] = "3-32 characters: latin letters, digits, underscore"
	} else if botNameRe.MatchString(req.Username) {
		errs["username"] = "this username is reserved"
	}

	if !validPassword(req.Password) {
		errs["password"] = "password must be 8-128 characters"
	}

	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	if utf8.RuneCountInString(req.FirstName) > 64 {
		errs["first_name"] = "max 64 characters"
	}
	if utf8.RuneCountInString(req.LastName) > 64 {
		errs["last_name"] = "max 64 characters"
	}
	return errs
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if !s.requireCaptcha(w, r, req.CaptchaToken) {
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	user, err := s.store.CreateUser(r.Context(), req.Email, req.Username, hash, req.FirstName, req.LastName)
	switch {
	case errors.Is(err, store.ErrEmailTaken):
		writeFieldErrors(w, http.StatusConflict, "conflict", "already registered",
			map[string]string{"email": "email already registered"})
		return
	case errors.Is(err, store.ErrUsernameTaken):
		writeFieldErrors(w, http.StatusConflict, "conflict", "already taken",
			map[string]string{"username": "username already taken"})
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}

	if err := s.recordConsent(r, user.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.log.Info("user registered", "user_id", user.ID)
	s.event(r, user.ID, "register")
	s.sendVerification(r, user)
	s.startSession(w, r, user, http.StatusCreated)
}

// handleLogin: шаг 1. Если включена 2FA, сессия не выдаётся,
// а возвращается mfa_token для шага 2 (POST /auth/2fa/verify).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	user, err := s.store.UserByEmail(r.Context(), normalizeEmail(req.Email))
	if errors.Is(err, store.ErrNotFound) {
		auth.VerifyDummy(req.Password) // выравниваем время ответа
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	if user.PasswordHash == store.BotPasswordHash { // бот: входа по паролю нет
		auth.VerifyDummy(req.Password)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		s.log.Warn("failed login", "user_id", user.ID, "ip", clientIP(r))
		s.event(r, user.ID, "login_failed")
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	// О бане сообщаем только после верного пароля, чтобы не раскрывать статус чужих аккаунтов.
	if user.BannedAt != nil {
		writeError(w, http.StatusForbidden, "banned", "account is banned")
		return
	}

	if user.TOTPEnabledAt != nil {
		mfa, err := s.tokens.IssueMFA(user.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": mfa})
		return
	}

	s.finishLogin(w, r, user)
}

// finishLogin: журнал, оповещение о входе с нового устройства и сессия.
func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request, user *store.User) {
	meta := sessionMeta(r)
	known, first, err := s.store.KnownDevice(r.Context(), user.ID, meta)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !known && !first {
		s.event(r, user.ID, "login_new_device")
		s.sendMail(user.Email, "Вход в аккаунт с нового устройства",
			"Здравствуйте, "+user.FirstName+"!\n\n"+
				"В ваш аккаунт выполнен вход с нового устройства.\n"+
				"Браузер: "+meta.UserAgent+"\nIP: "+ipString(meta.IP)+"\n\n"+
				"Если это были не вы, смените пароль и завершите все сеансы:\n"+
				s.cfg.AppURL+"/settings/security\n")
	} else {
		s.event(r, user.ID, "login")
	}
	s.startSession(w, r, user, http.StatusOK)
}

// handleMFAVerify: шаг 2 входа. {"mfa_token": "...", "code": "123456"}
// или {"mfa_token": "...", "backup_code": "abcd-efgh"}.
func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken   string `json:"mfa_token"`
		Code       string `json:"code"`
		BackupCode string `json:"backup_code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	userID, err := s.tokens.ParseMFA(req.MFAToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired mfa token")
		return
	}
	user, err := s.store.UserByID(r.Context(), userID)
	if err != nil || user.BannedAt != nil || user.TOTPEnabledAt == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired mfa token")
		return
	}
	ok, err := s.checkSecondFactor(r, user, req.Code, req.BackupCode)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_code", "invalid code")
		return
	}
	s.finishLogin(w, r, user)
}

// startSession выдаёт access-токен в теле ответа и refresh-токен в HttpOnly cookie.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user *store.User, status int) {
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sid, err := s.store.StartSession(r.Context(), user.ID, hash, s.cfg.RefreshTTL, sessionMeta(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	access, exp, err := s.tokens.IssueAccess(user.ID, sid)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.setRefreshCookie(w, refresh, s.cfg.RefreshTTL)
	self := toSelf(user)
	writeJSON(w, status, authResponse{AccessToken: access, TokenType: "Bearer", ExpiresAt: exp, User: &self})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing refresh token")
		return
	}

	newToken, newHash, err := auth.NewRefreshToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	userID, sid, err := s.store.RotateRefreshToken(r.Context(), auth.HashRefreshToken(c.Value), newHash, s.cfg.RefreshTTL, sessionMeta(r))
	switch {
	case errors.Is(err, store.ErrTokenReused):
		s.log.Warn("refresh token reuse: all sessions revoked", "user_id", userID, "ip", clientIP(r))
		s.event(r, userID, "refresh_token_reuse")
		if u, err := s.store.UserByID(r.Context(), userID); err == nil {
			s.sendMail(u.Email, "Подозрительная активность: все сеансы завершены",
				"Кто-то попытался использовать старый ключ сеанса вашего аккаунта. "+
					"Это бывает, если ключ был украден. На всякий случай мы завершили все сеансы.\n\n"+
					"Войдите заново и смените пароль: "+s.cfg.AppURL+"/settings/security\n")
		}
		s.clearRefreshCookie(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "session revoked, please log in again")
		return
	case errors.Is(err, store.ErrTokenInvalid):
		s.clearRefreshCookie(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired refresh token")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}

	access, exp, err := s.tokens.IssueAccess(userID, sid)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.setRefreshCookie(w, newToken, s.cfg.RefreshTTL)
	writeJSON(w, http.StatusOK, authResponse{AccessToken: access, TokenType: "Bearer", ExpiresAt: exp})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if err := s.store.RevokeRefreshToken(r.Context(), auth.HashRefreshToken(c.Value)); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Cookie видна только эндпоинтам /api/v1/auth, недоступна JS (HttpOnly)
// и не уходит с чужих сайтов (SameSite=Strict): это защита от XSS-кражи и CSRF.
func (s *Server) setRefreshCookie(w http.ResponseWriter, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- HttpOnly и SameSite=Strict всегда; Secure выключается только COOKIE_SECURE=false для разработки по http
		Name:     refreshCookie,
		Value:    value,
		Path:     "/api/v1/auth",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- HttpOnly и SameSite=Strict всегда; Secure выключается только COOKIE_SECURE=false для разработки по http
		Name:     refreshCookie,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func sessionMeta(r *http.Request) store.SessionMeta {
	m := store.SessionMeta{UserAgent: r.UserAgent()}
	if ip, err := netip.ParseAddr(clientIP(r)); err == nil {
		m.IP = &ip
	}
	return m
}

func ipString(ip *netip.Addr) string {
	if ip == nil {
		return "неизвестен"
	}
	return ip.String()
}

// internalError пишет подробности в лог, а клиенту отдаёт общий ответ
// без стектрейсов и текста ошибок БД.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	// клиент ушёл, не дождавшись ответа (закрыл вкладку, перешёл по ссылке) —
	// это не сбой сервера, будить дежурного незачем
	if r.Context().Err() != nil && errors.Is(err, context.Canceled) {
		s.log.Info("request canceled by client", "path", r.URL.Path)
		w.WriteHeader(499)
		return
	}
	s.log.Error("internal error", "err", err, "path", r.URL.Path)
	writeError(w, http.StatusInternalServerError, "internal", "internal server error")
}
