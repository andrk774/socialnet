package httpapi

// Сквозной тест на настоящем PostgreSQL. Запуск:
//   TEST_DATABASE_URL=postgres://user:pass@localhost:5432/socialnet_test?sslmode=disable go test ./...
// Без переменной тест пропускается.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"socialnet/internal/config"
	"socialnet/internal/db"
	"socialnet/internal/mailer"
	"socialnet/internal/store"
)

func setup(t *testing.T, burst int) *httptest.Server {
	t.Helper()
	ts, _, _ := setupFull(t, burst)
	return ts
}

// setupFull возвращает ещё сервер и «почтовый ящик», куда падают все письма.
func setupFull(t *testing.T, burst int) (*httptest.Server, *Server, *mailer.Memory) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil { // повторный запуск не должен падать
		t.Fatal(err)
	}

	cfg := config.Config{
		JWTSecret:  []byte(strings.Repeat("s", 32)),
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
		PlusPrice:  199,

		AuthRatePerMin: 1,
		AuthRateBurst:  burst,
		MediaDir:       t.TempDir(),
		TOTPKey:        []byte(strings.Repeat("t", 32)),
		AppURL:         "https://app.test",
	}
	log := slog.New(&failOnError{t: t})
	srv, err := New(cfg, store.New(pool), log)
	if err != nil {
		t.Fatal(err)
	}
	mbox := &mailer.Memory{}
	srv.mailer = mbox
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return ts, srv, mbox
}

type testClient struct {
	t    *testing.T
	base string
	http *http.Client
	ua   string // User-Agent, чтобы имитировать разные устройства
}

func newClient(t *testing.T, base string) *testClient {
	jar, _ := cookiejar.New(nil)
	return &testClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *testClient) do(method, path, token string, body any) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestAuthFlow(t *testing.T) {
	ts := setup(t, 1000)
	c := newClient(t, ts.URL)

	reg := map[string]any{
		"email": "Ivan@Example.com", "username": "ivan_p",
		"password": "s3cret-pass", "first_name": "Иван", "last_name": "Петров",
		"age_confirmed": true, "accept_terms": true, "pd_consent": true,
	}

	code, body := c.do("POST", "/api/v1/auth/register", "", reg)
	if code != http.StatusCreated {
		t.Fatalf("register: %d %v", code, body)
	}
	user := body["user"].(map[string]any)
	if user["email"] != "ivan@example.com" {
		t.Fatalf("email not normalized: %v", user["email"])
	}
	if _, leaked := user["password_hash"]; leaked {
		t.Fatal("password hash leaked in response")
	}
	access := body["access_token"].(string)

	// Повтор email (в другом регистре) и username
	reg2 := map[string]any{"email": "IVAN@example.com", "username": "other", "password": "s3cret-pass", "age_confirmed": true, "accept_terms": true, "pd_consent": true}
	if code, _ := c.do("POST", "/api/v1/auth/register", "", reg2); code != http.StatusConflict {
		t.Fatalf("duplicate email: %d", code)
	}
	reg3 := map[string]any{"email": "x@example.com", "username": "IVAN_P", "password": "s3cret-pass", "age_confirmed": true, "accept_terms": true, "pd_consent": true}
	if code, _ := c.do("POST", "/api/v1/auth/register", "", reg3); code != http.StatusConflict {
		t.Fatalf("duplicate username: %d", code)
	}

	// Валидация
	code, body = c.do("POST", "/api/v1/auth/register", "", map[string]any{"email": "bad", "username": "a", "password": "short"})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("validation: %d", code)
	}
	if f := body["error"].(map[string]any)["fields"].(map[string]any); len(f) != 6 {
		t.Fatalf("expected 6 field errors (3 fields + 3 consents), got %v", f)
	}

	// Лишнее поле (mass assignment) отвергается
	if code, _ := c.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "y@example.com", "username": "yyy", "password": "s3cret-pass", "is_admin": true,
	}); code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", code)
	}

	// /me
	code, body = c.do("GET", "/api/v1/users/me", access, nil)
	if code != http.StatusOK || body["username"] != "ivan_p" || body["first_name"] != "Иван" {
		t.Fatalf("me: %d %v", code, body)
	}
	if code, _ := c.do("GET", "/api/v1/users/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("me without token: %d", code)
	}
	if code, _ := c.do("GET", "/api/v1/users/me", access+"x", nil); code != http.StatusUnauthorized {
		t.Fatalf("me with tampered token: %d", code)
	}

	// Вход: неверный пароль и несуществующий email дают одинаковый ответ
	_, b1 := c.do("POST", "/api/v1/auth/login", "", map[string]any{"email": "ivan@example.com", "password": "wrong-pass"})
	_, b2 := c.do("POST", "/api/v1/auth/login", "", map[string]any{"email": "nobody@example.com", "password": "wrong-pass"})
	if b1["error"].(map[string]any)["message"] != b2["error"].(map[string]any)["message"] {
		t.Fatal("login errors differ: user enumeration")
	}

	code, body = c.do("POST", "/api/v1/auth/login", "", map[string]any{"email": "IVAN@example.com", "password": "s3cret-pass"})
	if code != http.StatusOK {
		t.Fatalf("login: %d %v", code, body)
	}

	// Refresh: ротация токена
	u, _ := http.NewRequest("GET", ts.URL+"/api/v1/auth/refresh", nil)
	oldCookie := c.http.Jar.Cookies(u.URL)[0].Value

	code, body = c.do("POST", "/api/v1/auth/refresh", "", nil)
	if code != http.StatusOK || body["access_token"] == "" {
		t.Fatalf("refresh: %d %v", code, body)
	}
	newCookie := c.http.Jar.Cookies(u.URL)[0].Value
	if newCookie == oldCookie {
		t.Fatal("refresh token not rotated")
	}

	// Reuse detection: старый токен предъявлен снова — все сессии отзываются
	attacker := newClient(t, ts.URL)
	attacker.http.Jar.SetCookies(u.URL, []*http.Cookie{{Name: refreshCookie, Value: oldCookie, Path: "/api/v1/auth"}})
	if code, _ := attacker.do("POST", "/api/v1/auth/refresh", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("reused token accepted: %d", code)
	}
	if code, _ := c.do("POST", "/api/v1/auth/refresh", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("legit session survived reuse detection: %d", code)
	}

	// Новый вход и logout
	c.do("POST", "/api/v1/auth/login", "", map[string]any{"email": "ivan@example.com", "password": "s3cret-pass"})
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/logout", nil)
	resp, err := c.http.Do(req)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: %v %v", resp.StatusCode, err)
	}
	resp.Body.Close()
	if code, _ := c.do("POST", "/api/v1/auth/refresh", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: %d", code)
	}
}

func TestRateLimit(t *testing.T) {
	ts := setup(t, 5)
	c := newClient(t, ts.URL)
	var got429 bool
	for i := 0; i < 15; i++ {
		code, _ := c.do("POST", "/api/v1/auth/login", "", map[string]any{"email": "a@b.cd", "password": "whatever1"})
		if code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("rate limit never triggered")
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts := setup(t, 5)
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}
}

// failOnError: любая запись уровня ERROR в логе сервера валит тест.
// Так не пропадают ошибки, которые API скрывает от клиента (500, уведомления).
type failOnError struct {
	t     *testing.T
	attrs []slog.Attr
}

func (h *failOnError) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelError }
func (h *failOnError) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool { msg += " " + a.String(); return true })
	h.t.Errorf("server logged error: %s", msg)
	return nil
}
func (h *failOnError) WithAttrs(a []slog.Attr) slog.Handler { return &failOnError{t: h.t, attrs: a} }
func (h *failOnError) WithGroup(string) slog.Handler        { return h }
