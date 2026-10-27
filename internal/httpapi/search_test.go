package httpapi

import (
	"testing"
	"time"
)

func TestSearchHiking(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	start := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-72 * time.Hour)
	anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Иремель зимой", "starts_at": start, "meeting_point": "Тюлюк"}, 201)
	anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Иремель летом", "starts_at": start}, 201)
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE events SET starts_at = $1 WHERE title = 'Иремель летом'`, past); err != nil { // прошёл
		t.Fatal(err)
	}
	anna.req("POST", p("/events"), map[string]any{"title": "Встреча про Иремель", "starts_at": start}, 201) // не поход
	anna.req("POST", p("/articles"), map[string]any{"title": "Как я ходил", "body": "Подъём на Иремель от Тюлюка", "status": "published"}, 201)
	anna.req("POST", p("/articles"), map[string]any{"title": "Иремель — черновик", "body": "x"}, 201)
	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	anna.req("PATCH", p("/routes/%s", route["id"]), map[string]any{"public": true, "public_note": "Кольцо на Иремель"}, 200)
	_, private := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	_ = private

	all := bob.req("GET", p("/search?q=%s&type=all", "иремел"), nil, 200)
	count := func(m map[string]any, k string) int { v, _ := m[k].([]any); return len(v) }
	if count(all, "hikes") != 1 || count(all, "articles") != 1 || count(all, "routes") != 1 {
		t.Fatalf("all: hikes %d articles %d routes %d", count(all, "hikes"), count(all, "articles"), count(all, "routes"))
	}
	if r := all["routes"].([]any)[0].(map[string]any); r["url"] != "/trails/"+route["id"].(string) || r["author"].(map[string]any)["username"] != "anna" {
		t.Errorf("route item: %v", r)
	}
	if _, ok := all["users"]; !ok {
		t.Error("all without users")
	}
	if n := len(items(bob.req("GET", p("/search?q=%s&type=hikes", "тюлюк"), nil, 200))); n != 1 {
		t.Errorf("hike by meeting point: %d", n)
	}
	if n := len(items(bob.req("GET", p("/search?q=%s&type=articles", "черновик"), nil, 200))); n != 0 {
		t.Errorf("draft found: %d", n)
	}
	if n := len(items(bob.req("GET", p("/search?q=%s&type=routes", "%25%25"), nil, 200))); n != 0 {
		t.Errorf("%% matched everything: %d", n)
	}
	bob.req("GET", p("/search?q=x&type=hikes"), nil, 400)
	bob.req("GET", p("/search?q=иремель&type=nope"), nil, 400)

	// заблокировавший не находится
	anna.req("POST", p("/users/%d/block", eve.ID), nil, 204)
	all = eve.req("GET", p("/search?q=%s&type=all", "иремел"), nil, 200)
	if count(all, "hikes") != 0 || count(all, "articles") != 0 {
		t.Errorf("blocked sees: hikes %d articles %d", count(all, "hikes"), count(all, "articles"))
	}
}
