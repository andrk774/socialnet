package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestQRLogin(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.cfg.AppURL = "https://hike.example"
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")

	// новое устройство: код и секрет опроса
	pc := newClient(t, ts.URL)
	pc.ua = "Mozilla/5.0 (Windows NT 10.0) Firefox/140.0"
	code, start := pc.do("POST", "/api/v1/auth/qr/start", "", nil)
	if code != 201 || start["poll_token"] == "" || !strings.HasPrefix(start["qr_svg"].(string), "<svg") ||
		start["url"] != "https://hike.example/qr-login?c="+start["id"].(string) {
		t.Fatalf("start: %d %v", code, start)
	}
	id, poll := start["id"].(string), start["poll_token"].(string)
	pollReq := func(c *testClient, tok string) (int, map[string]any) {
		return c.do("POST", "/api/v1/auth/qr/poll", "", map[string]any{"id": id, "poll_token": tok})
	}
	if code, out := pollReq(pc, poll); code != 202 || out["status"] != "pending" {
		t.Fatalf("pending: %d %v", code, out)
	}

	// телефон Анны видит устройство; без входа и токеном приложения — нельзя
	info := anna.req("GET", "/api/v1/qr-login/"+id, nil, 200)
	if !strings.Contains(info["user_agent"].(string), "Firefox") {
		t.Fatalf("info: %v", info)
	}
	if code, _ := newClient(t, ts.URL).do("POST", "/api/v1/qr-login/"+id+"/approve", "", nil); code != 401 {
		t.Fatalf("anonymous approve: %d", code)
	}
	anna.req("POST", "/api/v1/qr-login/"+id+"/approve", nil, 204)
	bob.req("POST", "/api/v1/qr-login/"+id+"/approve", nil, 410) // уже подтверждён

	// чужой секрет (например, увидели QR-код на экране) — не забрать
	if code, _ := pollReq(newClient(t, ts.URL), "wrong-secret"); code != 410 {
		t.Fatalf("wrong secret: %d", code)
	}
	code, out := pollReq(pc, poll)
	if code != 200 || out["user"].(map[string]any)["username"] != "anna" {
		t.Fatalf("claim: %d %v", code, out)
	}
	if code, me := pc.do("GET", "/api/v1/users/me", out["access_token"].(string), nil); code != 200 || me["username"] != "anna" {
		t.Fatalf("session: %d %v", code, me)
	}
	// сеанс с refresh-cookie: обновление работает
	if code, _ := pc.do("POST", "/api/v1/auth/refresh", "", nil); code != 200 {
		t.Fatalf("refresh: %d", code)
	}
	// один раз
	if code, _ := pollReq(pc, poll); code != 410 {
		t.Fatalf("second claim: %d", code)
	}
	// новое устройство в списке сеансов Анны, событие в журнале
	sessions := items(anna.req("GET", "/api/v1/sessions", nil, 200))
	var found bool
	for _, s := range sessions {
		found = found || strings.Contains(s.(map[string]any)["user_agent"].(string), "Firefox")
	}
	if !found {
		t.Errorf("no Firefox session: %v", sessions)
	}

	// отклонение
	_, s2 := pc.do("POST", "/api/v1/auth/qr/start", "", nil)
	bob.req("POST", "/api/v1/qr-login/"+s2["id"].(string)+"/reject", nil, 204)
	if code, _ := pc.do("POST", "/api/v1/auth/qr/poll", "", map[string]any{"id": s2["id"], "poll_token": s2["poll_token"]}); code != 403 {
		t.Fatalf("rejected: %d", code)
	}
	// истёкший
	_, s3 := pc.do("POST", "/api/v1/auth/qr/start", "", nil)
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE qr_logins SET expires_at = now() - interval '5 seconds' WHERE id = $1`, s3["id"]); err != nil {
		t.Fatal(err)
	}
	anna.req("GET", "/api/v1/qr-login/"+s3["id"].(string), nil, 410)
	anna.req("POST", "/api/v1/qr-login/"+s3["id"].(string)+"/approve", nil, 410)
	// подтверждён перед самым концом — забрать можно ещё минуту
	_, s4 := pc.do("POST", "/api/v1/auth/qr/start", "", nil)
	anna.req("POST", "/api/v1/qr-login/"+s4["id"].(string)+"/approve", nil, 204)
	_, _ = srv.store.Pool().Exec(t.Context(), `UPDATE qr_logins SET expires_at = now() - interval '5 seconds' WHERE id = $1`, s4["id"])
	if code, _ := pc.do("POST", "/api/v1/auth/qr/poll", "", map[string]any{"id": s4["id"], "poll_token": s4["poll_token"]}); code != 200 {
		t.Fatalf("approved just in time: %d", code)
	}

	// забаненный не войдёт, даже если подтвердил раньше
	_, s5 := pc.do("POST", "/api/v1/auth/qr/start", "", nil)
	bob.req("POST", "/api/v1/qr-login/"+s5["id"].(string)+"/approve", nil, 204)
	_ = srv.store.Ban(t.Context(), bob.ID, "test")
	if code, _ := pc.do("POST", "/api/v1/auth/qr/poll", "", map[string]any{"id": s5["id"], "poll_token": s5["poll_token"]}); code != http.StatusForbidden {
		t.Fatalf("banned: %d", code)
	}
}
