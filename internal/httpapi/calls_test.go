package httpapi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCallsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	if exec.Command(node, "-e", "require('playwright')").Run() != nil {
		t.Skip("playwright unavailable")
	}
	ts := setup(t, 1000)
	_, file, _, _ := runtime.Caller(0)
	shots := os.Getenv("E2E_SCREENSHOTS")
	if shots == "" {
		shots = t.TempDir()
	}
	if err := os.MkdirAll(shots, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, filepath.Join(filepath.Dir(file), "../../web/e2e/calls.mjs"), ts.URL, shots)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("calls browser: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func TestCalls(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	a, b, c := register(t, ts, "calla"), register(t, ts, "callb"), register(t, ts, "callc")
	offer := map[string]any{"video": true, "offer": "v=0\r\ntest-offer"}
	made := a.req("POST", p("/users/%d/calls", b.ID), offer, 201)
	id := int64(made["id"].(float64))
	c.req("GET", p("/calls/%d", id), nil, 404)
	c.req("POST", p("/calls/%d/action", id), map[string]any{"action": "end"}, 404)
	c.req("POST", p("/users/%d/calls", b.ID), offer, 409)
	a.req("POST", p("/calls/%d/action", id), map[string]any{"action": "answer", "answer": "v=0\r\nanswer"}, 409)
	if _, ok := items(b.req("GET", p("/calls"), nil, 200))[0].(map[string]any)["offer"]; ok {
		t.Fatal("SDP leaked into history")
	}
	b.req("POST", p("/calls/%d/action", id), map[string]any{"action": "answer", "answer": "v=0\r\nanswer"}, 204)
	b.req("POST", p("/calls/%d/action", id), map[string]any{"action": "answer", "answer": "v=0\r\nanswer"}, 409)
	a.req("POST", p("/calls/%d/action", id), map[string]any{"action": "heartbeat"}, 204)
	a.req("POST", p("/calls/%d/action", id), map[string]any{"action": "end"}, 204)
	ended := a.req("GET", p("/calls/%d", id), nil, 200)
	if ended["status"] != "ended" || ended["offer"] != nil || ended["answer"] != nil {
		t.Fatal("SDP not cleared", ended)
	}
	a.req("DELETE", p("/calls/history"), nil, 204)
	if len(items(a.req("GET", p("/calls"), nil, 200))) != 0 || len(items(b.req("GET", p("/calls"), nil, 200))) != 1 {
		t.Fatal("history clearing not isolated")
	}
	b.req("POST", p("/users/%d/block", a.ID), nil, 204)
	a.req("POST", p("/users/%d/calls", b.ID), offer, 403)
	b.req("DELETE", p("/users/%d/block", a.ID), nil, 204)
	made = a.req("POST", p("/users/%d/calls", b.ID), offer, 201)
	id = int64(made["id"].(float64))
	_, err := srv.store.Pool().Exec(context.Background(), `UPDATE calls SET created_at=now()-interval '2 minutes' WHERE id=$1`, id)
	if err != nil {
		t.Fatal(err)
	}
	expired := b.req("GET", p("/calls/%d", id), nil, 200)
	if expired["status"] != "missed" {
		t.Fatal(expired)
	}
	b.req("POST", p("/calls/%d/action", id), map[string]any{"action": "answer", "answer": "v=0\r\nanswer"}, 409)
	a.req("POST", p("/users/%d/calls", a.ID), offer, 403)
}
