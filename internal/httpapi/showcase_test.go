package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestShowcase(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.publicLimiter = newIPRateLimiter(100, 100)
	anna := register(t, ts, "anna")
	start := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	get := func() (int, map[string]any) {
		resp, err := http.Get(ts.URL + "/api/v1/showcase")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if code, out := get(); code != 200 || len(out["hikes"].([]any)) != 0 {
		t.Fatalf("empty showcase: %d %v", code, out)
	}
	anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": start,
		"meeting_point": "у подъезда дома 5", "description": "звоните +7 900"}, 201)
	// кеш: минута
	if _, out := get(); len(out["hikes"].([]any)) != 0 {
		t.Fatal("showcase not cached")
	}
	srv.showcase.ttl = time.Nanosecond
	_, out := get()
	hikes := out["hikes"].([]any)
	if len(hikes) != 1 {
		t.Fatalf("hikes: %v", out)
	}
	h := hikes[0].(map[string]any)
	if h["title"] != "Таганай" || h["going_count"].(float64) != 1 {
		t.Errorf("hike: %v", h)
	}
	for _, private := range []string{"meeting_point", "description", "creator_id", "gear"} {
		if _, ok := h[private]; ok {
			t.Errorf("guest sees %s", private)
		}
	}
	// закрытое сообщество — не в витрине
	c := anna.req("POST", p("/communities"), map[string]any{"name": "Свои", "slug": "svoi", "visibility": "closed"}, 201)
	anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Секретный", "starts_at": start, "community_id": c["id"]}, 201)
	if _, out := get(); len(out["hikes"].([]any)) != 1 {
		t.Errorf("closed community hike in showcase: %v", out["hikes"])
	}
}
