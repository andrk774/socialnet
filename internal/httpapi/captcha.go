package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Капча (Яндекс SmartCaptcha) на регистрации и сбросе пароля — там, где
// бот может завести тысячи аккаунтов или рассылать письма на чужие адреса.
// Ключи — CAPTCHA_SITE_KEY и CAPTCHA_SECRET; без них капча выключена.
//
// Если сам сервис капчи недоступен (сеть, не 200), запрос пропускается:
// так советует документация SmartCaptcha, чтобы люди не упирались в сбой.
// Защиту в этот момент держат лимиты по IP; сбой виден в журнале и метрике.

const captchaDomain = "https://smartcaptcha.cloud.yandex.ru"

type captchaResult int

const (
	captchaOK captchaResult = iota
	captchaFailed
	captchaUnavailable
)

func (s *Server) captchaEnabled() bool { return s.cfg.CaptchaSiteKey != "" && s.cfg.CaptchaSecret != "" }

func (s *Server) checkCaptcha(ctx context.Context, token, ip string) captchaResult {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 {
		return captchaFailed
	}
	form := url.Values{"secret": {s.cfg.CaptchaSecret}, "token": {token}}
	if ip != "" {
		form.Set("ip", ip)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.CaptchaURL, strings.NewReader(form.Encode())) // #nosec G704 -- адрес из настроек администратора
	if err != nil {
		return captchaUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.captchaClient.Do(req) // #nosec G704 -- см. выше
	if err != nil {
		s.log.Warn("captcha unavailable", "err", err)
		return captchaUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		s.log.Warn("captcha unavailable", "status", resp.StatusCode)
		return captchaUnavailable
	}
	var out struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return captchaUnavailable
	}
	if out.Status == "ok" {
		return captchaOK
	}
	if out.Message != "" {
		s.log.Info("captcha rejected", "message", out.Message) // только для диагностики
	}
	return captchaFailed
}

// requireCaptcha: false — ответ уже отправлен.
func (s *Server) requireCaptcha(w http.ResponseWriter, r *http.Request, token string) bool {
	if !s.captchaEnabled() {
		return true
	}
	switch s.checkCaptcha(r.Context(), token, clientIP(r)) {
	case captchaOK:
		s.m.business.Inc("captcha_ok")
		return true
	case captchaUnavailable:
		s.m.business.Inc("captcha_unavailable")
		return true
	}
	s.m.business.Inc("captcha_failed")
	writeFieldErrors(w, http.StatusUnprocessableEntity, "captcha_failed", "captcha check failed",
		map[string]string{"captcha_token": "confirm you are not a robot"})
	return false
}
