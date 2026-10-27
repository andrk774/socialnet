package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCaptcha(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	var mode atomic.Value
	mode.Store("ok")
	var gotSecret, gotIP atomic.Value
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotSecret.Store(r.PostForm.Get("secret"))
		gotIP.Store(r.PostForm.Get("ip"))
		switch {
		case mode.Load() == "down":
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.PostForm.Get("token") == "good-token" && mode.Load() == "ok":
			_, _ = w.Write([]byte(`{"status":"ok","message":"","host":"hike.example"}`))
		default:
			_, _ = w.Write([]byte(`{"status":"failed","message":"Invalid or expired Token."}`))
		}
	}))
	defer fake.Close()
	srv.cfg.CaptchaSiteKey, srv.cfg.CaptchaSecret, srv.cfg.CaptchaURL = "site-key", "server-secret", fake.URL+"/validate"

	reg := func(name, token string) int {
		code, _ := newClient(t, ts.URL).do("POST", "/api/v1/auth/register", "", map[string]any{
			"email": name + "@example.com", "username": name, "password": "password-" + name,
			"age_confirmed": true, "accept_terms": true, "pd_consent": true, "captcha_token": token})
		return code
	}
	if code := reg("bot1", ""); code != 422 {
		t.Fatalf("no token: %d", code)
	}
	if code := reg("bot2", "forged"); code != 422 {
		t.Fatalf("bad token: %d", code)
	}
	if code := reg("human", "good-token"); code != 201 {
		t.Fatalf("good token: %d", code)
	}
	if gotSecret.Load() != "server-secret" || gotIP.Load() == "" {
		t.Errorf("validation request: secret=%v ip=%v", gotSecret.Load(), gotIP.Load())
	}
	// сервис капчи лёг — пропускаем (рекомендация SmartCaptcha), держат лимиты
	mode.Store("down")
	if code := reg("during_outage", "whatever"); code != 201 {
		t.Fatalf("captcha outage blocks people: %d", code)
	}
	mode.Store("ok")
	// сброс пароля тоже под капчей
	c := newClient(t, ts.URL)
	if code, _ := c.do("POST", "/api/v1/auth/password/forgot", "", map[string]any{"email": "human@example.com"}); code != 422 {
		t.Fatalf("forgot without captcha: %d", code)
	}
	// клиент знает ключ сайта, CSP пускает виджет
	if code, cfg := c.do("GET", "/api/v1/client-config", "", nil); code != 200 || cfg["captcha"].(map[string]any)["site_key"] != "site-key" {
		t.Fatalf("client-config: %v", cfg)
	}
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src https://smartcaptcha.cloud.yandex.ru") ||
		!strings.Contains(csp, "script-src 'self' https://smartcaptcha.cloud.yandex.ru") {
		t.Fatalf("csp: %s", csp)
	}
}
