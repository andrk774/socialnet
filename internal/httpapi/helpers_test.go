package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tuser — зарегистрированный тестовый пользователь со своим клиентом.
type tuser struct {
	*testClient
	ID       int64
	Username string
	Token    string
}

func register(t *testing.T, ts *httptest.Server, username string) *tuser {
	t.Helper()
	c := newClient(t, ts.URL)
	code, body := c.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": username + "@example.com", "username": username,
		"password": "password-" + username, "first_name": username,
	})
	if code != http.StatusCreated {
		t.Fatalf("register %s: %d %v", username, code, body)
	}
	u := body["user"].(map[string]any)
	return &tuser{testClient: c, ID: int64(u["id"].(float64)), Username: username, Token: body["access_token"].(string)}
}

// req: запрос от имени пользователя, ожидаем код want.
func (u *tuser) req(method, path string, body any, want int) map[string]any {
	u.t.Helper()
	code, out := u.do(method, path, u.Token, body)
	if code != want {
		u.t.Fatalf("%s %s as %s: got %d, want %d: %v", method, path, u.Username, code, want, out)
	}
	return out
}

func p(format string, args ...any) string { return fmt.Sprintf("/api/v1"+format, args...) }

func items(m map[string]any) []any {
	v, _ := m["items"].([]any)
	return v
}

func befriend(t *testing.T, a, b *tuser) {
	t.Helper()
	a.req("POST", p("/users/%d/friend", b.ID), nil, http.StatusOK)
	b.req("POST", p("/users/%d/friend", a.ID), nil, http.StatusOK)
}
