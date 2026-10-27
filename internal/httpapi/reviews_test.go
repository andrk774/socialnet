package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReviews(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	org, bob, carol, mod := register(t, ts, "org"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	start := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	h := org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Иремель", "starts_at": start}, 201)
	hid := int64(h["id"].(float64))
	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)

	// до похода — рано
	bob.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 5}, 409)
	if got := bob.req("GET", p("/events/%d", hid), nil, 200); got["can_review"] == true {
		t.Fatal("can review before the hike")
	}
	setStart := func(at time.Time) {
		if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE events SET starts_at = $2 WHERE id = $1`, hid, at); err != nil {
			t.Fatal(err)
		}
	}
	setStart(time.Now().Add(-30 * time.Hour))

	if got := bob.req("GET", p("/events/%d", hid), nil, 200); got["can_review"] != true || got["my_review"] != nil {
		t.Fatalf("review state: %v %v", got["can_review"], got["my_review"])
	}
	bob.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 0}, 422)
	bob.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 6}, 422)
	org.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 5}, 403)   // о себе
	carol.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 1}, 403) // не ходила
	bob.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 4, "text": "Шли бодро, всё объясняли"}, 200)
	if !has(notifTypes(t, org, hid), "hike_review") {
		t.Error("organizer not notified")
	}
	// исправить — та же запись
	bob.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 5, "text": "Отлично"}, 200)
	rv := carol.req("GET", p("/users/%d/reviews", org.ID), nil, 200)
	if r := rv["rating"].(map[string]any); r["avg"].(float64) != 5 || r["count"].(float64) != 1 {
		t.Fatalf("rating: %v", rv)
	}
	item := items(rv)[0].(map[string]any)
	if item["text"] != "Отлично" || item["event_title"] != "Иремель" || item["author"].(map[string]any)["username"] != "bob" {
		t.Fatalf("review: %v", item)
	}
	if got := bob.req("GET", p("/events/%d", hid), nil, 200); got["my_review"].(map[string]any)["rating"].(float64) != 5 {
		t.Fatalf("my review: %v", got["my_review"])
	}

	// через 90 дней — поздно
	setStart(time.Now().Add(-91 * 24 * time.Hour))
	bob.req("POST", p("/events/%d/review", hid), map[string]any{"rating": 1}, 409)

	// маршрут каталога
	_, route := uploadFile(t, ts, org, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	rid := route["id"].(string)
	carol.req("POST", p("/routes/%s/reviews", rid), map[string]any{"rating": 4}, 404) // ещё не в каталоге
	org.req("PATCH", p("/routes/%s", rid), map[string]any{"public": true}, 200)
	org.req("POST", p("/routes/%s/reviews", rid), map[string]any{"rating": 5}, 403) // свой
	carol.req("POST", p("/routes/%s/reviews", rid), map[string]any{"rating": 3, "text": "Курумник мокрый, осторожно"}, 200)
	bob.req("POST", p("/routes/%s/reviews", rid), map[string]any{"rating": 5}, 200)
	rr := bob.req("GET", p("/routes/%s/reviews", rid), nil, 200)
	if r := rr["rating"].(map[string]any); r["avg"].(float64) != 4 || r["count"].(float64) != 2 {
		t.Fatalf("route rating: %v", rr)
	}
	// оценка видна и на открытой странице маршрута
	srv.publicLimiter = newIPRateLimiter(100, 100)
	if resp, err := http.Get(ts.URL + "/trails/" + rid); err != nil {
		t.Fatal(err)
	} else {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), "оценка 4.0 из 5 (2)") {
			t.Errorf("trail page has no rating")
		}
	}

	// жалоба и снятие модератором — из оценки пропадает
	cid := int64(items(rr)[1].(map[string]any)["id"].(float64))
	bob.req("POST", p("/reports"), map[string]any{"target_type": "review", "target_id": cid, "reason": "неправда"}, 201)
	var rep map[string]any
	for _, it := range items(mod.req("GET", p("/mod/reports?status=open"), nil, 200)) {
		if r := it.(map[string]any); r["target_type"] == "review" {
			rep = r
		}
	}
	if tg, _ := rep["target"].(map[string]any); tg == nil || tg["snippet"] != "Курумник мокрый, осторожно" {
		t.Fatalf("review report target: %v", rep)
	}
	mod.req("POST", p("/mod/reports/%v/resolve", rep["id"]), map[string]any{"action": "hide"}, 204)
	if r := bob.req("GET", p("/routes/%s/reviews", rid), nil, 200)["rating"].(map[string]any); r["count"].(float64) != 1 {
		t.Errorf("hidden review counted: %v", r)
	}

	// удалить свой
	bob.req("DELETE", p("/routes/%s/reviews", rid), nil, 204)
	bob.req("DELETE", p("/routes/%s/reviews", rid), nil, 404)
	bob.req("DELETE", p("/events/%d/review", hid), nil, 204)
	if r := carol.req("GET", p("/users/%d/reviews", org.ID), nil, 200)["rating"].(map[string]any); r["count"].(float64) != 0 {
		t.Errorf("deleted review counted: %v", r)
	}
}
