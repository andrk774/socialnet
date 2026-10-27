package httpapi

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRides(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	drv, bob, carol, dan, eve := register(t, ts, "drv"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "dan"), register(t, ts, "eve")
	at := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	h := drv.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": at}, 201)
	hid := int64(h["id"].(float64))

	for name, bad := range map[string]map[string]any{
		"seats":    {"seats": 9},
		"past":     {"departs_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)},
		"far":      {"departs_at": time.Now().Add(90 * 24 * time.Hour).UTC().Format(time.RFC3339)},
		"no from":  {"from": " "},
		"no event": {"event_id": 999999},
	} {
		body := map[string]any{"from": "Миасс, автовокзал", "to": "Таганай, Белый ключ", "departs_at": at, "seats": 2}
		for k, v := range bad {
			body[k] = v
		}
		if code, out := drv.do("POST", p("/rides"), drv.Token, body); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}
	ride := drv.req("POST", p("/rides"), map[string]any{"from": "Миасс, автовокзал", "to": "Таганай, Белый ключ", "departs_at": at,
		"seats": 2, "event_id": hid, "note": "Нива, багажник небольшой"}, 201)
	rid := int64(ride["id"].(float64))

	if n := len(items(bob.req("GET", p("/rides?event_id=%d", hid), nil, 200))); n != 1 {
		t.Fatalf("rides for hike: %d", n)
	}
	if n := len(items(bob.req("GET", p("/rides?to=таган"), nil, 200))); n != 1 {
		t.Fatalf("rides by destination: %d", n)
	}
	drv.req("POST", p("/rides/%d/request", rid), map[string]any{}, 400)
	if got := bob.req("POST", p("/rides/%d/request", rid), map[string]any{"note": "Возьму термос"}, 200); got["my_status"] != "pending" {
		t.Fatalf("request: %v", got)
	}
	bob.req("POST", p("/rides/%d/request", rid), map[string]any{}, 200) // повтор ничего не меняет
	carol.req("POST", p("/rides/%d/request", rid), map[string]any{}, 200)
	dan.req("POST", p("/rides/%d/request", rid), map[string]any{}, 200)
	if !has(rideNotifs(t, drv, rid), "ride_request") {
		t.Error("driver not notified")
	}
	bob.req("GET", p("/rides/%d/requests", rid), nil, 403)
	reqs := items(drv.req("GET", p("/rides/%d/requests", rid), nil, 200))
	if len(reqs) != 3 {
		t.Fatalf("requests: %v", reqs)
	}
	bob.req("POST", p("/rides/%d/requests/%d", rid, carol.ID), map[string]any{"accept": true}, 403)
	drv.req("POST", p("/rides/%d/requests/%d", rid, bob.ID), map[string]any{"accept": true}, 200)
	got := drv.req("POST", p("/rides/%d/requests/%d", rid, carol.ID), map[string]any{"accept": true}, 200)
	if got["taken"].(float64) != 2 || got["pending"].(float64) != 1 {
		t.Fatalf("seats: %v", got)
	}
	drv.req("POST", p("/rides/%d/requests/%d", rid, dan.ID), map[string]any{"accept": true}, 409) // мест нет
	if !has(rideNotifs(t, bob, rid), "ride_accepted") {
		t.Error("bob not notified")
	}
	// попутчик передумал — место свободно, водитель узнаёт
	carol.req("DELETE", p("/rides/%d/request", rid), nil, 204)
	if !has(rideNotifs(t, drv, rid), "ride_left") {
		t.Error("driver not told about leaving")
	}
	drv.req("POST", p("/rides/%d/requests/%d", rid, dan.ID), map[string]any{"accept": true}, 200)

	// заблокированный водителем не видит поездку
	drv.req("POST", p("/users/%d/block", eve.ID), nil, 204)
	eve.req("GET", p("/rides/%d", rid), nil, 404)
	eve.req("POST", p("/rides/%d/request", rid), map[string]any{}, 404)

	// отмена поездки — попутчикам уведомление
	bob.req("DELETE", p("/rides/%d", rid), nil, 403)
	drv.req("DELETE", p("/rides/%d", rid), nil, 204)
	if !hasType(t, dan, "ride_canceled") {
		t.Error("dan not told about cancellation")
	}
}

// Последнее место при одновременном принятии получает один.
func TestRideLastSeat(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	drv := register(t, ts, "drv")
	at := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	ride := drv.req("POST", p("/rides"), map[string]any{"from": "Миасс", "to": "Иремель", "departs_at": at, "seats": 1}, 201)
	rid := int64(ride["id"].(float64))
	var users []*tuser
	for i := range 6 {
		u := register(t, ts, fmt.Sprintf("pass%d", i))
		u.req("POST", p("/rides/%d/request", rid), map[string]any{}, 200)
		users = append(users, u)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for _, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _ := drv.do("POST", p("/rides/%d/requests/%d", rid, u.ID), drv.Token, map[string]any{"accept": true}); code == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("accepted %d for one seat", ok)
	}
}

func rideNotifs(t *testing.T, u *tuser, ride int64) []string {
	t.Helper()
	var out []string
	for _, it := range items(u.req("GET", p("/notifications?limit=50"), nil, 200)) {
		n := it.(map[string]any)
		if id, _ := n["ride_id"].(float64); int64(id) == ride {
			out = append(out, n["type"].(string))
		}
	}
	return out
}

func hasType(t *testing.T, u *tuser, typ string) bool {
	t.Helper()
	for _, it := range items(u.req("GET", p("/notifications?limit=50"), nil, 200)) {
		if it.(map[string]any)["type"] == typ {
			return true
		}
	}
	return false
}
