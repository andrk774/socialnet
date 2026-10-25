package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/auth"
	"socialnet/internal/mailer"
	"socialnet/internal/store"
)

const (
	verifyEmailTTL   = 48 * time.Hour
	resetPasswordTTL = time.Hour
)

// event пишет запись в журнал безопасности пользователя.
func (s *Server) event(r *http.Request, userID int64, typ string) {
	s.m.events.Inc(typ)
	if err := s.store.AddSecurityEvent(r.Context(), userID, typ, sessionMeta(r)); err != nil {
		s.log.Error("security event", "err", err, "type", typ)
	}
}

// sendMail отправляет письмо в фоне. Так время ответа не зависит от того,
// существует ли адрес (иначе по задержке можно перебирать зарегистрированные email).
func (s *Server) sendMail(to, subject, body string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.mailer.Send(ctx, mailer.Message{To: to, Subject: subject, Body: body}); err != nil {
			s.log.Warn("send mail failed", "err", err, "subject", subject)
		}
	}()
}

func (s *Server) sendVerification(r *http.Request, user *store.User) {
	token, hash, err := auth.NewEmailToken()
	if err != nil {
		s.log.Error("email token", "err", err)
		return
	}
	if err := s.store.CreateAuthToken(r.Context(), user.ID, "verify_email", hash, verifyEmailTTL); err != nil {
		s.log.Error("email token", "err", err)
		return
	}
	s.sendMail(user.Email, "Подтвердите email",
		"Здравствуйте, "+user.FirstName+"!\n\nПодтвердите адрес, перейдя по ссылке (действует 48 часов):\n"+
			s.cfg.AppURL+"/verify-email?token="+url.QueryEscape(token)+"\n\n"+
			"Если вы не регистрировались, просто проигнорируйте письмо.\n")
}

// ---------- email ----------

func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	user, err := s.store.UserByID(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if user.EmailVerifiedAt != nil {
		writeError(w, http.StatusConflict, "already_verified", "email already verified")
		return
	}
	if !s.mailLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "try again later")
		return
	}
	s.sendVerification(r, user)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	userID, err := s.store.ConsumeAuthToken(r.Context(), "verify_email", auth.HashRefreshToken(req.Token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_token", "link is invalid or expired")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.MarkEmailVerified(r.Context(), userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, userID, "email_verified")
	w.WriteHeader(http.StatusNoContent)
}

// ---------- пароль ----------

// handleForgotPassword всегда отвечает 202, есть такой email или нет.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.store.UserByEmail(r.Context(), normalizeEmail(req.Email))
	if err == nil && user.BannedAt == nil {
		token, hash, terr := auth.NewEmailToken()
		if terr == nil {
			terr = s.store.CreateAuthToken(r.Context(), user.ID, "reset_password", hash, resetPasswordTTL)
		}
		if terr != nil {
			s.internalError(w, r, terr)
			return
		}
		s.sendMail(user.Email, "Сброс пароля",
			"Кто-то (надеемся, вы) запросил сброс пароля.\n\nСсылка действует 1 час:\n"+
				s.cfg.AppURL+"/reset-password?token="+url.QueryEscape(token)+"\n\n"+
				"Если это были не вы, ничего не делайте: пароль останется прежним.\n")
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "if this email is registered, a reset link has been sent",
	})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validPassword(req.Password) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"password": "password must be 8-128 characters"})
		return
	}
	userID, err := s.store.ConsumeAuthToken(r.Context(), "reset_password", auth.HashRefreshToken(req.Token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_token", "link is invalid or expired")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !s.setPassword(w, r, userID, req.Password) {
		return
	}
	// Сброс пароля — повод выкинуть все сеансы: возможно, аккаунт был взломан.
	if err := s.store.RevokeUserOAuthTokens(r.Context(), userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.RevokeAllSessions(r.Context(), userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	// Владение почтой подтверждено ссылкой из письма.
	_ = s.store.MarkEmailVerified(r.Context(), userID)
	s.event(r, userID, "password_reset")
	s.passwordChangedMail(userID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	v := viewerFrom(r.Context())
	if !s.checkPassword(w, r, v.ID, req.CurrentPassword) {
		return
	}
	if !validPassword(req.NewPassword) {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"new_password": "password must be 8-128 characters"})
		return
	}
	if !s.setPassword(w, r, v.ID, req.NewPassword) {
		return
	}
	// остальные устройства и сторонние приложения выходят, текущее остаётся
	if err := s.store.RevokeUserOAuthTokens(r.Context(), v.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.RevokeOtherSessions(r.Context(), v.ID, v.SID); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, v.ID, "password_changed")
	s.passwordChangedMail(v.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request, userID int64, password string) bool {
	hash, err := auth.HashPassword(password)
	if err == nil {
		err = s.store.SetPassword(r.Context(), userID, hash)
	}
	if err != nil {
		s.internalError(w, r, err)
		return false
	}
	return true
}

func (s *Server) passwordChangedMail(userID int64) {
	u, err := s.store.UserByID(context.Background(), userID)
	if err != nil {
		return
	}
	s.sendMail(u.Email, "Пароль изменён",
		"Пароль вашего аккаунта только что был изменён.\n\n"+
			"Если это были не вы, срочно восстановите доступ: "+s.cfg.AppURL+"/forgot-password\n")
}

// checkPassword: для опасных действий (2FA, смена пароля) просим пароль ещё раз.
// Украденного access-токена тогда недостаточно, чтобы захватить аккаунт.
func (s *Server) checkPassword(w http.ResponseWriter, r *http.Request, userID int64, password string) bool {
	if !s.mfaLimiter.allow("pw:" + strconv.FormatInt(userID, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return false
	}
	user, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		s.internalError(w, r, err)
		return false
	}
	ok, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		s.internalError(w, r, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "invalid_password", "wrong password")
		return false
	}
	return true
}

// ---------- 2FA ----------

// checkSecondFactor проверяет TOTP-код или резервный код.
// Не больше 5 попыток за 5 минут на пользователя: перебор шести цифр невозможен.
func (s *Server) checkSecondFactor(r *http.Request, user *store.User, code, backup string) (bool, error) {
	if !s.mfaLimiter.allow("2fa:" + strconv.FormatInt(user.ID, 10)) {
		return false, nil
	}
	ok := false
	switch {
	case code != "" && user.TOTPSecretEnc != nil:
		secret, err := s.box.Open(user.TOTPSecretEnc)
		if err != nil {
			return false, err
		}
		step, valid := auth.VerifyTOTP(string(secret), code, time.Now(), user.TOTPLastStep)
		if valid {
			if ok, err = s.store.AdvanceTOTPStep(r.Context(), user.ID, step); err != nil {
				return false, err
			}
		}
	case backup != "":
		var err error
		if ok, err = s.store.UseBackupCode(r.Context(), user.ID, auth.HashBackupCode(backup)); err != nil {
			return false, err
		}
		if ok {
			s.event(r, user.ID, "backup_code_used")
		}
	}
	if !ok {
		s.event(r, user.ID, "2fa_failed")
	}
	return ok, nil
}

func (s *Server) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	if !s.checkPassword(w, r, me, req.Password) {
		return
	}
	user, err := s.store.UserByID(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	sealed, err := s.box.Seal([]byte(secret))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	err = s.store.SetPendingTOTP(r.Context(), me, sealed)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "already_enabled", "two-factor authentication is already enabled")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret":      secret,
		"otpauth_uri": auth.TOTPURI("Socialnet", user.Username, secret), // для QR-кода
	})
}

func (s *Server) handle2FAEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	me := userIDFrom(r.Context())
	user, err := s.store.UserByID(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if user.TOTPEnabledAt != nil {
		writeError(w, http.StatusConflict, "already_enabled", "two-factor authentication is already enabled")
		return
	}
	if user.TOTPSecretEnc == nil {
		writeError(w, http.StatusBadRequest, "setup_required", "call /users/me/2fa/setup first")
		return
	}
	if !s.mfaLimiter.allow("2fa:" + strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	secret, err := s.box.Open(user.TOTPSecretEnc)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	step, ok := auth.VerifyTOTP(string(secret), req.Code, time.Now(), 0)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_code", "invalid code")
		return
	}
	codes, hashes, err := auth.NewBackupCodes()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.EnableTOTP(r.Context(), me, step, hashes); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, me, "2fa_enabled")
	s.sendMail(user.Email, "Двухфакторная аутентификация включена",
		"Для вашего аккаунта включена двухфакторная аутентификация.\n\n"+
			"Если это были не вы, срочно восстановите доступ: "+s.cfg.AppURL+"/forgot-password\n")
	// резервные коды показываются один раз
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

type sensitive2FARequest struct {
	Password   string `json:"password"`
	Code       string `json:"code"`
	BackupCode string `json:"backup_code"`
}

// confirm2FA: пароль + действующий второй фактор.
func (s *Server) confirm2FA(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	var req sensitive2FARequest
	if !decodeJSON(w, r, &req) {
		return nil, false
	}
	me := userIDFrom(r.Context())
	if !s.checkPassword(w, r, me, req.Password) {
		return nil, false
	}
	user, err := s.store.UserByID(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if user.TOTPEnabledAt == nil {
		writeError(w, http.StatusConflict, "not_enabled", "two-factor authentication is not enabled")
		return nil, false
	}
	ok, err := s.checkSecondFactor(r, user, req.Code, req.BackupCode)
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_code", "invalid code")
		return nil, false
	}
	return user, true
}

func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	user, ok := s.confirm2FA(w, r)
	if !ok {
		return
	}
	if err := s.store.DisableTOTP(r.Context(), user.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, user.ID, "2fa_disabled")
	s.sendMail(user.Email, "Двухфакторная аутентификация отключена",
		"Для вашего аккаунта отключена двухфакторная аутентификация.\n\n"+
			"Если это были не вы, срочно восстановите доступ: "+s.cfg.AppURL+"/forgot-password\n")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBackupCodes(w http.ResponseWriter, r *http.Request) {
	user, ok := s.confirm2FA(w, r)
	if !ok {
		return
	}
	codes, hashes, err := auth.NewBackupCodes()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.store.ReplaceBackupCodes(r.Context(), user.ID, hashes); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, user.ID, "backup_codes_regenerated")
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// ---------- сеансы и журнал ----------

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	v := viewerFrom(r.Context())
	items, err := s.store.Sessions(r.Context(), v.ID, v.SID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	sid := strings.ToLower(chi.URLParam(r, "sid"))
	if !validUUID(sid) {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	me := userIDFrom(r.Context())
	ok, err := s.store.RevokeSession(r.Context(), me, sid)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	s.event(r, me, "session_revoked")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	v := viewerFrom(r.Context())
	if err := s.store.RevokeOtherSessions(r.Context(), v.ID, v.SID); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, v.ID, "sessions_revoked")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSecurityEvents(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.SecurityEvents(r.Context(), userIDFrom(r.Context()), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(e store.SecurityEvent) int64 { return e.ID }))
}
