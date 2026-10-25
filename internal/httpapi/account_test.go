package httpapi

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"socialnet/internal/auth"
	"socialnet/internal/mailer"
)

var tokenInMail = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// waitMail ждёт письмо с темой, содержащей subject (письма уходят в фоне).
func waitMail(t *testing.T, box *mailer.Memory, to, subject string) mailer.Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range box.Sent() {
			if m.To == to && strings.Contains(m.Subject, subject) {
				return m
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no mail %q to %s; got %v", subject, to, box.Sent())
	return mailer.Message{}
}

func mailCount(box *mailer.Memory, to, subject string) int {
	n := 0
	for _, m := range box.Sent() {
		if m.To == to && strings.Contains(m.Subject, subject) {
			n++
		}
	}
	return n
}

func login(t *testing.T, ts *httptest.Server, email, password, ua string) (*testClient, map[string]any, int) {
	t.Helper()
	c := newClient(t, ts.URL)
	c.ua = ua
	code, body := c.do("POST", "/api/v1/auth/login", "", map[string]any{"email": email, "password": password})
	return c, body, code
}

func TestEmailVerification(t *testing.T) {
	ts, _, box := setupFull(t, 1000)
	anna := register(t, ts, "anna")

	m := waitMail(t, box, "anna@example.com", "Подтвердите email")
	token := tokenInMail.FindStringSubmatch(m.Body)[1]
	if !strings.Contains(m.Body, "https://app.test/verify-email?token=") {
		t.Fatalf("link: %s", m.Body)
	}

	anna.req("POST", p("/auth/email/verify"), map[string]any{"token": "wrong"}, 400)
	anna.req("POST", p("/auth/email/verify"), map[string]any{"token": token}, 204)
	anna.req("POST", p("/auth/email/verify"), map[string]any{"token": token}, 400) // одноразовый
	if me := anna.req("GET", p("/users/me"), nil, 200); me["email_verified"] != true {
		t.Fatal("not verified")
	}
	anna.req("POST", p("/users/me/email/verification"), nil, 409)
}

func TestPasswordReset(t *testing.T) {
	ts, _, box := setupFull(t, 1000)
	anna := register(t, ts, "anna")

	// одинаковый ответ для существующего и несуществующего адреса
	out1 := anna.req("POST", p("/auth/password/forgot"), map[string]any{"email": "anna@example.com"}, 202)
	out2 := anna.req("POST", p("/auth/password/forgot"), map[string]any{"email": "ghost@example.com"}, 202)
	if out1["message"] != out2["message"] {
		t.Fatal("responses differ: user enumeration")
	}
	m := waitMail(t, box, "anna@example.com", "Сброс пароля")
	token := tokenInMail.FindStringSubmatch(m.Body)[1]
	time.Sleep(50 * time.Millisecond)
	if mailCount(box, "ghost@example.com", "Сброс") != 0 {
		t.Fatal("mail sent to unknown address")
	}

	anna.req("POST", p("/auth/password/reset"), map[string]any{"token": token, "password": "short"}, 422)
	anna.req("POST", p("/auth/password/reset"), map[string]any{"token": token, "password": "brand-new-pass"}, 204)
	anna.req("POST", p("/auth/password/reset"), map[string]any{"token": token, "password": "another-pass"}, 400)

	// старая сессия завершена сразу, а не через 15 минут
	if code, _ := anna.do("GET", "/api/v1/users/me", anna.Token, nil); code != 401 {
		t.Fatalf("old session alive: %d", code)
	}
	if _, _, code := login(t, ts, "anna@example.com", "password-anna", ""); code != 401 {
		t.Fatal("old password works")
	}
	if _, _, code := login(t, ts, "anna@example.com", "brand-new-pass", ""); code != 200 {
		t.Fatal("new password rejected")
	}
	waitMail(t, box, "anna@example.com", "Пароль изменён")
}

func TestChangePasswordAndSessions(t *testing.T) {
	ts, _, box := setupFull(t, 1000)
	anna := register(t, ts, "anna")
	bob := register(t, ts, "bob")
	laptop, body, _ := login(t, ts, "anna@example.com", "password-anna", "Laptop/1.0")
	laptopToken := body["access_token"].(string)
	waitMail(t, box, "anna@example.com", "нового устройства")

	sessions := anna.req("GET", p("/sessions"), nil, 200)["items"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("sessions: %v", sessions)
	}
	var otherSID string
	for _, s := range sessions {
		m := s.(map[string]any)
		if m["current"] == false {
			otherSID = m["id"].(string)
		}
	}

	// чужую сессию завершить нельзя
	bob.req("DELETE", p("/sessions/%s", otherSID), nil, 404)
	if code, _ := laptop.do("GET", "/api/v1/users/me", laptopToken, nil); code != 200 {
		t.Fatal("laptop session killed by stranger")
	}

	anna.req("DELETE", p("/sessions/%s", otherSID), nil, 204)
	if code, _ := laptop.do("GET", "/api/v1/users/me", laptopToken, nil); code != 401 {
		t.Fatal("revoked session still works")
	}
	if code, _ := laptop.do("POST", "/api/v1/auth/refresh", "", nil); code != 401 {
		t.Fatal("revoked session can refresh")
	}

	// смена пароля: остальные устройства выходят, текущее остаётся
	phone, body, _ := login(t, ts, "anna@example.com", "password-anna", "Phone/2.0")
	phoneToken := body["access_token"].(string)
	anna.req("POST", p("/users/me/password"), map[string]any{"current_password": "wrong", "new_password": "new-password-1"}, 403)
	anna.req("POST", p("/users/me/password"), map[string]any{"current_password": "password-anna", "new_password": "new-password-1"}, 204)
	if code, _ := phone.do("GET", "/api/v1/users/me", phoneToken, nil); code != 401 {
		t.Fatal("other device survived password change")
	}
	anna.req("GET", p("/users/me"), nil, 200)

	// журнал безопасности
	types := map[string]bool{}
	for _, e := range items(anna.req("GET", p("/security/events"), nil, 200)) {
		types[e.(map[string]any)["type"].(string)] = true
	}
	for _, want := range []string{"register", "login_new_device", "session_revoked", "password_changed"} {
		if !types[want] {
			t.Errorf("missing event %s in %v", want, types)
		}
	}
}

func Test2FA(t *testing.T) {
	ts, srv, box := setupFull(t, 1000)
	srv.mfaLimiter = newIPRateLimiter(100, 100) // лимит проверяем отдельным тестом
	anna := register(t, ts, "anna")

	anna.req("POST", p("/users/me/2fa/setup"), map[string]any{"password": "wrong"}, 403)
	setup := anna.req("POST", p("/users/me/2fa/setup"), map[string]any{"password": "password-anna"}, 200)
	secret := setup["secret"].(string)
	if !strings.HasPrefix(setup["otpauth_uri"].(string), "otpauth://totp/") {
		t.Fatalf("uri: %v", setup)
	}

	anna.req("POST", p("/users/me/2fa/enable"), map[string]any{"code": "000000"}, 401)
	now := time.Now()
	code, _ := auth.TOTPCode(secret, now)
	enabled := anna.req("POST", p("/users/me/2fa/enable"), map[string]any{"code": code}, 200)
	backups := enabled["backup_codes"].([]any)
	if len(backups) != 10 {
		t.Fatalf("backup codes: %v", enabled)
	}
	waitMail(t, box, "anna@example.com", "включена")
	anna.req("POST", p("/users/me/2fa/setup"), map[string]any{"password": "password-anna"}, 409)

	// секрет в базе зашифрован
	var raw []byte
	_ = srv.store.Pool().QueryRow(t.Context(), `SELECT totp_secret_enc FROM users WHERE id = $1`, anna.ID).Scan(&raw)
	if strings.Contains(string(raw), secret) {
		t.Fatal("TOTP secret stored in plaintext")
	}

	// вход: без кода сессии нет
	c, body, st := login(t, ts, "anna@example.com", "password-anna", "")
	if st != 200 || body["mfa_required"] != true || body["access_token"] != nil {
		t.Fatalf("login with 2fa: %d %v", st, body)
	}
	mfa := body["mfa_token"].(string)
	// mfa_token нельзя использовать как обычный токен
	if code, _ := c.do("GET", "/api/v1/users/me", mfa, nil); code != 401 {
		t.Fatal("mfa token accepted as access token")
	}
	// тот же код, что при включении, повторно не принимается
	if st, _ := c.do("POST", "/api/v1/auth/2fa/verify", "", map[string]any{"mfa_token": mfa, "code": code}); st != 401 {
		t.Fatal("replayed TOTP accepted")
	}
	next, _ := auth.TOTPCode(secret, now.Add(30*time.Second))
	st, body = c.do("POST", "/api/v1/auth/2fa/verify", "", map[string]any{"mfa_token": mfa, "code": next})
	if st != 200 || body["access_token"] == nil {
		t.Fatalf("2fa verify: %d %v", st, body)
	}

	// резервный код одноразовый
	c2, body, _ := login(t, ts, "anna@example.com", "password-anna", "")
	mfa2 := body["mfa_token"].(string)
	bc := strings.ToUpper(backups[0].(string)) // регистр и дефис не важны
	if st, _ := c2.do("POST", "/api/v1/auth/2fa/verify", "", map[string]any{"mfa_token": mfa2, "backup_code": bc}); st != 200 {
		t.Fatal("backup code rejected")
	}
	if st, _ := c2.do("POST", "/api/v1/auth/2fa/verify", "", map[string]any{"mfa_token": mfa2, "backup_code": bc}); st != 401 {
		t.Fatal("backup code reused")
	}

	// отключение: нужен пароль и второй фактор
	anna.req("POST", p("/users/me/2fa/disable"), map[string]any{"password": "password-anna", "code": "123456"}, 401)
	anna.req("POST", p("/users/me/2fa/disable"), map[string]any{"password": "password-anna", "backup_code": backups[1]}, 204)
	if _, body, _ := login(t, ts, "anna@example.com", "password-anna", ""); body["access_token"] == nil {
		t.Fatal("2fa still required after disable")
	}
}

func Test2FABruteForceLimit(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	anna := register(t, ts, "anna")
	secret := anna.req("POST", p("/users/me/2fa/setup"), map[string]any{"password": "password-anna"}, 200)["secret"].(string)
	code, _ := auth.TOTPCode(secret, time.Now())
	anna.req("POST", p("/users/me/2fa/enable"), map[string]any{"code": code}, 200)

	c, body, _ := login(t, ts, "anna@example.com", "password-anna", "")
	mfa := body["mfa_token"].(string)
	for i := 0; i < 5; i++ {
		c.do("POST", "/api/v1/auth/2fa/verify", "", map[string]any{"mfa_token": mfa, "code": "000000"})
	}
	good, _ := auth.TOTPCode(secret, time.Now().Add(30*time.Second))
	if st, _ := c.do("POST", "/api/v1/auth/2fa/verify", "", map[string]any{"mfa_token": mfa, "code": good}); st != http.StatusUnauthorized {
		t.Fatal("brute-force limit not applied")
	}
}
