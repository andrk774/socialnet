package httpapi

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"socialnet/internal/redisx"
)

func testRedis(t *testing.T) (string, *redisx.Client) {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opt, err := redisx.ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	rc := redisx.New(opt)
	t.Cleanup(rc.Close)
	if _, err := rc.Do(t.Context(), "FLUSHDB"); err != nil {
		t.Fatal(err)
	}
	return raw, rc
}

// setupCluster — два экземпляра API на одной базе и одном Redis.
func setupCluster(t *testing.T) (a, b *httptest.Server, srvA, srvB *Server) {
	t.Helper()
	redisURL, _ := testRedis(t)
	_, base, _ := setupFull(t, 1000) // готовит чистую базу
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var out []*httptest.Server
	var srvs []*Server
	for i := 0; i < 2; i++ {
		cfg := base.cfg
		cfg.RedisURL = redisURL
		srv, err := New(cfg, base.store, slog.New(&failOnError{t: t}))
		if err != nil {
			t.Fatal(err)
		}
		srv.mailer = base.mailer
		srv.Start(ctx)
		ts := httptest.NewServer(srv.Router())
		t.Cleanup(ts.Close)
		out, srvs = append(out, ts), append(srvs, srv)
	}
	return out[0], out[1], srvs[0], srvs[1]
}

// on — тот же пользователь, но запросы идут на другой экземпляр.
func on(t *testing.T, ts *httptest.Server, u *tuser) *tuser {
	return &tuser{testClient: newClient(t, ts.URL), ID: u.ID, Username: u.Username + "@" + ts.URL, Token: u.Token}
}

func TestClusterRealtimeAcrossInstances(t *testing.T) {
	tsA, tsB, srvA, _ := setupCluster(t)
	anna := register(t, tsA, "anna")
	bob := register(t, tsB, "bob")

	// тикет выдан экземпляром A, подключение — к B; второй раз тикет не годится
	ticket := on(t, tsA, bob).req("POST", p("/ws/ticket"), nil, 200)["ticket"].(string)
	wsURL := func(ts *httptest.Server) string {
		return strings.Replace(ts.URL, "http", "ws", 1) + "/api/v1/ws?ticket=" + ticket
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(tsB), nil)
	if err != nil {
		t.Fatalf("ticket from A rejected by B: %v", err)
	}
	defer conn.CloseNow()
	if c2, _, err := websocket.Dial(ctx, wsURL(tsA), nil); err == nil {
		c2.CloseNow()
		t.Fatal("ticket used twice")
	}

	// A узнаёт, что Борис онлайн на B, и доставляет туда сообщение
	deadline := time.Now().Add(5 * time.Second)
	for !srvA.hub.Online(bob.ID) {
		if time.Now().After(deadline) {
			t.Fatal("bob never seen online from A")
		}
		time.Sleep(50 * time.Millisecond)
	}
	on(t, tsA, anna).req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": "Привет через Redis"}, 201)
	if ev := waitEvent(t, conn, "message"); ev["body"] != "Привет через Redis" {
		t.Fatalf("event: %v", ev)
	}
	// отключился — другие экземпляры больше не считают его онлайн
	conn.Close(websocket.StatusNormalClosure, "")
	for srvA.hub.Online(bob.ID) {
		if time.Now().After(deadline.Add(5 * time.Second)) {
			t.Fatal("bob still online after disconnect")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestClusterSharedRateLimit(t *testing.T) {
	_, rc := testRedis(t)
	log := slog.New(&failOnError{t: t})
	a := newRedisLimiter(rc, log, "test", 1.0/60, 3)
	b := newRedisLimiter(rc, log, "test", 1.0/60, 3)
	got := []bool{a.allow("ip1"), b.allow("ip1"), a.allow("ip1"), b.allow("ip1"), a.allow("ip2")}
	if want := []bool{true, true, true, false, true}; strings.Join(fmtBools(got), ",") != strings.Join(fmtBools(want), ",") {
		t.Fatalf("shared limit: %v", got)
	}

	// Redis недоступен — лимит считается локально, запросы не падают
	dead := redisx.New(redisx.Options{Addr: "127.0.0.1:1"})
	l := newRedisLimiter(dead, slog.New(slog.DiscardHandler), "x", 1.0/60, 2)
	first, second, third := l.allow("k"), l.allow("k"), l.allow("k")
	if !first || !second || third {
		t.Fatal("local fallback limit broken")
	}
}

func fmtBools(b []bool) []string {
	out := make([]string, len(b))
	for i, v := range b {
		out[i] = map[bool]string{true: "1", false: "0"}[v]
	}
	return out
}
