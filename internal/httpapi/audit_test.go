package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"socialnet/internal/store"
)

func TestAuditLogAndAnomalyDetector(t *testing.T) {
	ts, srv, mbox := setupFull(t, 1000)
	admin, anna, eve := register(t, ts, "admin"), register(t, ts, "anna"), register(t, ts, "eve")
	pool := srv.store.Pool()
	if _, err := pool.Exec(t.Context(), `UPDATE users SET role = 'admin' WHERE id = $1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	srv.cfg.SecurityContact = "security@example.com"

	// действия админа попадают в журнал вместе с телом запроса и результатом
	admin.req("POST", p("/admin/wallet/mint"), map[string]any{"user_id": anna.ID, "amount": 100000, "idempotency_key": "mint-1"}, 201)
	admin.req("POST", p("/mod/users/%d/ban", eve.ID), map[string]any{"reason": "спам"}, 204)
	admin.req("POST", p("/mod/users/%d/ban", 999999), map[string]any{"reason": "нет такого"}, 404)
	anna.req("GET", p("/admin/audit"), nil, 403)

	entries := items(admin.req("GET", p("/admin/audit"), nil, 200))
	if len(entries) < 3 {
		t.Fatalf("audit entries: %v", entries)
	}
	var mint map[string]any
	for _, e := range entries {
		if m := e.(map[string]any); m["action"] == "POST /api/v1/admin/wallet/mint" {
			mint = m
		}
	}
	if mint == nil || mint["status"].(float64) != 201 || mint["actor_id"].(float64) != float64(admin.ID) ||
		mint["details"].(map[string]any)["amount"].(float64) != 100000 || mint["ip"] != "127.0.0.1" {
		t.Fatalf("mint entry: %v", mint)
	}
	if v := admin.req("GET", p("/admin/audit/verify"), nil, 200); v["ok"] != true || v["checked"].(float64) < 3 {
		t.Fatalf("verify: %v", v)
	}

	// журнал только дописывается — даже напрямую в базе
	if _, err := pool.Exec(t.Context(), `UPDATE audit_log SET status = 200`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("update allowed: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM audit_log`); err == nil {
		t.Fatal("delete allowed")
	}
	if _, err := pool.Exec(t.Context(), `TRUNCATE audit_log`); err == nil {
		t.Fatal("truncate allowed")
	}

	// аномалии: перебор пароля одного аккаунта, «распыление» по многим
	for i := 0; i < 10; i++ {
		newClient(t, ts.URL).do("POST", "/api/v1/auth/login", "", map[string]any{"email": "anna@example.com", "password": "wrong-password"})
	}
	for _, u := range []string{"user1", "user2", "user3", "user4"} {
		register(t, ts, u)
		newClient(t, ts.URL).do("POST", "/api/v1/auth/login", "", map[string]any{"email": u + "@example.com", "password": "wrong-password"})
	}
	// админ действует с нового адреса
	if err := srv.store.AppendAudit(t.Context(), store.AuditEntry{ActorID: admin.ID, ActorRole: "admin",
		Action: "POST /api/v1/admin/users/{id}/role", Target: "/api/v1/admin/users/2/role", Status: 200, IP: "203.0.113.7"}); err != nil {
		t.Fatal(err)
	}

	alerts, err := srv.RunDetectorOnce(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, a := range alerts {
		got[a.Rule] = a.Subject
	}
	if got["login_bruteforce"] != "user:"+strconv.FormatInt(anna.ID, 10) || got["password_spray"] != "ip:127.0.0.1" ||
		got["mint_large"] != "mint" || got["admin_new_ip"] != "actor:"+strconv.FormatInt(admin.ID, 10)+":203.0.113.7" {
		t.Fatalf("alerts: %v", got)
	}
	if again, _ := srv.RunDetectorOnce(t.Context()); len(again) != 0 {
		t.Fatalf("duplicate alerts: %v", again)
	}
	open := items(admin.req("GET", p("/admin/alerts"), nil, 200))
	if len(open) != len(alerts) {
		t.Fatalf("open alerts: %d", len(open))
	}
	admin.req("POST", p("/admin/alerts/%d/ack", int64(open[0].(map[string]any)["id"].(float64))), nil, 204)
	if n := len(items(admin.req("GET", p("/admin/alerts"), nil, 200))); n != len(alerts)-1 {
		t.Fatalf("after ack: %d", n)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		sent := mbox.Sent()
		if len(sent) > 0 && sent[len(sent)-1].To == "security@example.com" && strings.Contains(sent[len(sent)-1].Body, "password_spray") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no alert email to security contact")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// подмена записи в обход триггера обнаруживается проверкой цепочки
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE audit_log DISABLE TRIGGER audit_log_no_update;
		UPDATE audit_log SET details = '{"amount": 1}' WHERE action = 'POST /api/v1/admin/wallet/mint';
		ALTER TABLE audit_log ENABLE TRIGGER audit_log_no_update;`); err != nil {
		t.Fatal(err)
	}
	srv.log = slog.New(slog.DiscardHandler) // «audit chain broken» здесь ожидаем
	if v := admin.req("GET", p("/admin/audit/verify"), nil, 200); v["ok"] != false || v["broken_at"].(float64) != mint["id"].(float64) {
		t.Fatalf("tampering not detected: %v", v)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna := register(t, ts, "anna")
	anna.req("GET", p("/users/%d", anna.ID), nil, 200)
	anna.req("GET", p("/users/%d", 424242), nil, 404)
	newClient(t, ts.URL).do("POST", "/api/v1/auth/login", "", map[string]any{"email": "anna@example.com", "password": "nope-nope"})

	scrape := func(token string) (int, string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/metrics", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		srv.MetricsHandler().ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	code, out := scrape("")
	for _, want := range []string{
		`socialnet_http_requests_total{method="GET",route="/api/v1/users/{id}",status="200"} 1`,
		`socialnet_http_requests_total{method="GET",route="/api/v1/users/{id}",status="404"} 1`,
		`socialnet_http_requests_total{method="POST",route="/api/v1/auth/login",status="401"} 1`,
		`socialnet_security_events_total{type="login_failed"} 1`,
		`socialnet_http_request_duration_seconds_count{method="POST",route="/api/v1/auth/register"} 1`,
		`socialnet_queue_depth{queue="jobs"} 0`,
		`socialnet_db_pool_connections{state="max"}`,
		`socialnet_ws_connections 0`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	if code != 200 || strings.Contains(out, "424242") {
		t.Fatal("raw ids leaked into metric labels")
	}
	srv.cfg.MetricsToken = "scrape-secret"
	if code, _ := scrape(""); code != http.StatusUnauthorized {
		t.Fatalf("metrics without token: %d", code)
	}
	if code, _ := scrape("scrape-secret"); code != 200 {
		t.Fatal("metrics with token")
	}
}
