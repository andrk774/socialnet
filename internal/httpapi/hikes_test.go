package httpapi

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func notifTypes(t *testing.T, u *tuser, event int64) []string {
	t.Helper()
	var out []string
	for _, it := range items(u.req("GET", p("/notifications?limit=50"), nil, 200)) {
		n := it.(map[string]any)
		if id, _ := n["event_id"].(float64); int64(id) == event {
			out = append(out, n["type"].(string))
		}
	}
	return out
}

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestHikes(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	org, bob, carol, dan, eve := register(t, ts, "org"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "dan"), register(t, ts, "eve")
	start := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)

	code, route := uploadFile(t, ts, org, "/api/v1/routes", "taganay.gpx", []byte(hikeGPX()), nil)
	if code != 201 {
		t.Fatalf("route: %d %v", code, route)
	}
	rid := route["id"].(string)
	_, foreign := uploadFile(t, ts, bob, "/api/v1/routes", "bob.gpx", []byte(hikeGPX()), nil)

	// проверки полей
	for name, bad := range map[string]map[string]any{
		"difficulty": {"difficulty": 6}, "capacity": {"capacity": 0}, "kind": {"kind": "party"},
		"route_id": {"route_id": "nope"}, "foreign route": {"route_id": foreign["id"]},
	} {
		body := map[string]any{"title": "Таганай", "starts_at": start, "kind": "hike"}
		for k, v := range bad {
			body[k] = v
		}
		if code, out := org.do("POST", p("/events"), org.Token, body); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}

	// поход на 3 места (организатор занимает одно) без одобрения
	h := org.req("POST", p("/events"), map[string]any{
		"kind": "hike", "title": "Двуглавая сопка", "starts_at": start, "difficulty": 2, "capacity": 3,
		"gear": "Треккинговые ботинки\nДождевик\n2 л воды", "route_id": rid, "meeting_point": "Ж/д вокзал Златоуста, 7:00",
	}, 201)
	hid := int64(h["id"].(float64))
	if h["kind"] != "hike" || h["going_count"].(float64) != 1 || h["route_id"] != rid || h["my_status"] != "going" {
		t.Fatalf("created: %v", h)
	}
	// маршрут похода видят все, кто видит поход, даже без записи
	carol.req("GET", p("/routes/%s", rid), nil, 200)
	if n := len(items(carol.req("GET", p("/hikes"), nil, 200))); n != 1 {
		t.Fatalf("hikes list: %d", n)
	}
	if n := len(items(carol.req("GET", p("/hikes?max_difficulty=1"), nil, 200))); n != 0 {
		t.Fatalf("difficulty filter: %d", n)
	}
	carol.req("GET", p("/hikes?max_difficulty=9"), nil, 400)

	// «возможно» в походе не бывает
	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "maybe"}, 422)
	if out := bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200); out["status"] != "going" {
		t.Fatalf("bob: %v", out)
	}
	if out := carol.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200); out["status"] != "going" {
		t.Fatalf("carol: %v", out)
	}
	if out := dan.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200); out["status"] != "waitlist" {
		t.Fatalf("dan should wait: %v", out)
	}
	if out := eve.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200); out["status"] != "waitlist" {
		t.Fatalf("eve should wait: %v", out)
	}
	// повторное «иду» не сдвигает очередь
	dan.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)
	if got := eve.req("GET", p("/events/%d", hid), nil, 200); got["my_waitlist_position"].(float64) != 2 || got["waitlist_count"].(float64) != 2 {
		t.Fatalf("eve position: %v", got)
	}
	if n := len(items(org.req("GET", p("/events/%d/attendees?status=waitlist", hid), nil, 200))); n != 2 {
		t.Fatalf("waitlist: %d", n)
	}

	// Боб уходит — место первому в очереди (Дэну), Дэну уведомление
	bob.req("DELETE", p("/events/%d/rsvp", hid), nil, 204)
	if got := dan.req("GET", p("/events/%d", hid), nil, 200); got["my_status"] != "going" {
		t.Fatalf("dan not promoted: %v", got)
	}
	if !has(notifTypes(t, dan, hid), "event_seat") {
		t.Error("dan not notified about the seat")
	}
	if got := eve.req("GET", p("/events/%d", hid), nil, 200); got["my_waitlist_position"].(float64) != 1 {
		t.Fatalf("eve moved up: %v", got)
	}

	// организатор не уходит и не удаляет себя; чужие не управляют
	org.req("DELETE", p("/events/%d/rsvp", hid), nil, 422)
	org.req("DELETE", p("/events/%d/attendees/%d", hid, org.ID), nil, 422)
	carol.req("DELETE", p("/events/%d/attendees/%d", hid, dan.ID), nil, 403)
	carol.req("GET", p("/events/%d/applications", hid), nil, 403)
	carol.req("PATCH", p("/events/%d", hid), map[string]any{"capacity": 10}, 403)

	// организатор убирает Кэрол — место Еве
	org.req("DELETE", p("/events/%d/attendees/%d", hid, carol.ID), nil, 204)
	if !has(notifTypes(t, carol, hid), "hike_removed") {
		t.Error("carol not notified")
	}
	if got := eve.req("GET", p("/events/%d", hid), nil, 200); got["my_status"] != "going" {
		t.Fatalf("eve not promoted: %v", got)
	}

	// больше мест — очередь переходит сама; null снимает ограничение
	carol.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200) // снова в очередь
	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)
	upd := org.req("PATCH", p("/events/%d", hid), map[string]any{"capacity": 4}, 200)
	if upd["going_count"].(float64) != 4 || upd["waitlist_count"].(float64) != 1 || upd["gear"] == "" || upd["difficulty"].(float64) != 2 {
		t.Fatalf("capacity 4: %v", upd)
	}
	upd = org.req("PATCH", p("/events/%d", hid), map[string]any{"capacity": nil, "difficulty": nil}, 200)
	if upd["going_count"].(float64) != 5 || upd["waitlist_count"].(float64) != 0 || upd["capacity"] != nil || upd["difficulty"] != nil {
		t.Fatalf("unlimited: %v", upd)
	}
	org.req("PATCH", p("/events/%d", hid), map[string]any{"kind": "event"}, 422)
	org.req("PATCH", p("/events/%d", hid), map[string]any{"route_id": foreign["id"]}, 422)
	if upd = org.req("PATCH", p("/events/%d", hid), map[string]any{"route_id": nil}, 200); upd["route_id"] != nil {
		t.Fatalf("route detach: %v", upd)
	}
}

func TestHikeApplications(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	org, bob, carol, dan := register(t, ts, "org"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "dan")
	start := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	h := org.req("POST", p("/events"), map[string]any{
		"kind": "hike", "title": "Иремель", "starts_at": start, "difficulty": 4, "capacity": 2, "approval": true,
	}, 201)
	hid := int64(h["id"].(float64))

	note := "Ходил на Иремель зимой, есть кошки"
	if out := bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going", "note": note}, 200); out["status"] != "pending" {
		t.Fatalf("bob: %v", out)
	}
	carol.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)
	if !has(notifTypes(t, org, hid), "hike_application") {
		t.Error("organizer not notified about an application")
	}
	apps := items(org.req("GET", p("/events/%d/applications", hid), nil, 200))
	if len(apps) != 2 || apps[0].(map[string]any)["note"] != note {
		t.Fatalf("applications: %v", apps)
	}
	// заявки не занимают места и не видны в списках участников
	if got := org.req("GET", p("/events/%d", hid), nil, 200); got["going_count"].(float64) != 1 || got["pending_count"].(float64) != 2 {
		t.Fatalf("counts: %v", got)
	}
	org.req("GET", p("/events/%d/attendees?status=pending", hid), nil, 400)

	org.req("POST", p("/events/%d/applications/%d", hid, bob.ID), map[string]any{}, 422)
	if out := org.req("POST", p("/events/%d/applications/%d", hid, bob.ID), map[string]any{"approve": true}, 200); out["status"] != "going" {
		t.Fatalf("approve bob: %v", out)
	}
	org.req("POST", p("/events/%d/applications/%d", hid, bob.ID), map[string]any{"approve": true}, 404) // уже решено
	org.req("POST", p("/events/%d/applications/%d", hid, dan.ID), map[string]any{"approve": true}, 404) // не подавал
	if !has(notifTypes(t, bob, hid), "hike_approved") {
		t.Error("bob not notified")
	}
	// мест больше нет: одобренная заявка уходит в очередь
	if out := org.req("POST", p("/events/%d/applications/%d", hid, carol.ID), map[string]any{"approve": true}, 200); out["status"] != "waitlist" {
		t.Fatalf("approve carol when full: %v", out)
	}
	if out := dan.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200); out["status"] != "pending" {
		t.Fatalf("dan: %v", out)
	}
	if out := org.req("POST", p("/events/%d/applications/%d", hid, dan.ID), map[string]any{"approve": false}, 200); out["status"] != "rejected" {
		t.Fatalf("reject: %v", out)
	}
	if !has(notifTypes(t, dan, hid), "hike_rejected") {
		t.Error("dan not notified about rejection")
	}
	if got := dan.req("GET", p("/events/%d", hid), nil, 200); got["my_status"] != nil {
		t.Fatalf("rejected still listed: %v", got)
	}
	// одобренный ушёл — место очереди (Кэрол), без повторного одобрения
	bob.req("DELETE", p("/events/%d/rsvp", hid), nil, 204)
	if got := carol.req("GET", p("/events/%d", hid), nil, 200); got["my_status"] != "going" {
		t.Fatalf("carol not promoted: %v", got)
	}

	// заблокировавший организатора не вступает
	dan.req("POST", p("/users/%d/block", org.ID), nil, 204)
	dan.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 403)
}

// Последнее место при одновременных заявках получает ровно один.
func TestHikeLastSeatRace(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	org := register(t, ts, "org")
	start := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	h := org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": start, "capacity": 2}, 201)
	hid := int64(h["id"].(float64))
	users := make([]*tuser, 8)
	for i := range users {
		users[i] = register(t, ts, fmt.Sprintf("walker%d", i))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	got := map[string]int{}
	for _, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, out := u.do("POST", p("/events/%d/rsvp", hid), u.Token, map[string]any{"status": "going"})
			if code != 200 {
				t.Errorf("rsvp: %d %v", code, out)
				return
			}
			mu.Lock()
			got[out["status"].(string)]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if got["going"] != 1 || got["waitlist"] != 7 {
		t.Fatalf("seats: %v", got)
	}
}
