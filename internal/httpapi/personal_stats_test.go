package httpapi

import (
	"testing"
	"time"
)

func TestPersonalStats(t *testing.T) {
	ts := setup(t, 1000)
	a, b := register(t, ts, "statsa"), register(t, ts, "statsb")
	created := a.req("POST", p("/posts"), map[string]any{"text": "Private statistics"}, 201)
	id := int64(created["id"].(float64))
	b.req("POST", p("/posts/%d/reactions", id), map[string]any{"reaction": "love"}, 200)
	a.req("POST", p("/posts/%d/comments", id), map[string]any{"text": "My exported comment"}, 201)
	if len(items(a.req("GET", p("/users/me/comments"), nil, 200))) != 1 {
		t.Fatal("missing own comment")
	}
	if len(items(b.req("GET", p("/users/me/comments"), nil, 200))) != 0 {
		t.Fatal("comments leaked")
	}
	a.req("GET", p("/users/me/comments?after=-1"), nil, 400)
	today := time.Now().UTC().Format("2006-01-02")
	url := p("/users/me/statistics?start=%s&end=%s", today, today)
	result := items(a.req("GET", url, nil, 200))
	if len(result) != 1 || result[0].(map[string]any)["likes"] != float64(1) {
		t.Fatal(result)
	}
	_, photo := upload(t, ts, a, pngBytes(20, 20))
	story := a.req("POST", p("/stories"), map[string]any{"photo_id": photo["id"]}, 201)
	sid := int64(story["id"].(float64))
	b.req("POST", p("/stories/%d/view", sid), nil, 204)
	b.req("POST", p("/stories/%d/view", sid), nil, 204)
	a.req("POST", p("/stories/%d/view", sid), nil, 204)
	storyRows := items(a.req("GET", url, nil, 200))
	found := false
	for _, raw := range storyRows {
		row := raw.(map[string]any)
		if row["kind"] == "story" {
			found = true
			if row["viewers"] != float64(1) {
				t.Fatal("view deduplication or own-view exclusion", row)
			}
		}
	}
	if !found {
		t.Fatal("missing story statistics")
	}
	if len(items(b.req("GET", url, nil, 200))) != 0 {
		t.Fatal("another user's statistics exposed")
	}
	if len(items(a.req("GET", p("/users/me/statistics?start=2000-01-01&end=2000-01-02"), nil, 200))) != 0 {
		t.Fatal("date filter ignored")
	}
	a.req("GET", p("/users/me/statistics?start=bad&end=bad"), nil, 400)
	a.req("GET", p("/users/me/statistics?start=2026-02-01&end=2026-01-01"), nil, 400)
	a.req("GET", p("/users/me/statistics?start=2000-01-01&end=2026-01-01"), nil, 400)
}
