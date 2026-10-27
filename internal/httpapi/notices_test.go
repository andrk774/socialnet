package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAreaNotices(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	srv.publicLimiter = newIPRateLimiter(100, 100)
	anna, mod := register(t, ts, "anna"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	day := func(n int) string { return time.Now().In(uralTime).AddDate(0, 0, n).Format(time.DateOnly) }

	// маршрут возле Таганая (hikeGPX — 55.05…55.07, 60.1)
	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	rid := route["id"].(string)
	anna.req("PATCH", p("/routes/%s", rid), map[string]any{"public": true}, 200)

	fire := map[string]any{"kind": "fire", "title": "Запрет посещения лесов", "body": "Особый противопожарный режим",
		"source_url": "https://example.gov.ru/fire", "bbox": []float64{54.9, 59.9, 55.2, 60.3}, "starts_on": day(-1), "ends_on": day(10)}
	anna.req("POST", p("/mod/notices"), fire, 403)
	for name, bad := range map[string]map[string]any{
		"kind":     {"kind": "rain"},
		"title":    {"title": " "},
		"bbox len": {"bbox": []float64{1, 2, 3}},
		"bbox min": {"bbox": []float64{55.2, 59.9, 54.9, 60.3}},
		"too big":  {"bbox": []float64{40, 40, 60, 60}},
		"url":      {"source_url": "javascript:alert(1)"},
		"dates":    {"ends_on": day(-5)},
		"date fmt": {"starts_on": "01.06.2027"},
	} {
		body := map[string]any{}
		for k, v := range fire {
			body[k] = v
		}
		for k, v := range bad {
			body[k] = v
		}
		if code, out := mod.do("POST", p("/mod/notices"), mod.Token, body); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}
	n := mod.req("POST", p("/mod/notices"), fire, 201)
	// закрытая тропа в другом районе и прошедшее предупреждение — не про этот маршрут
	mod.req("POST", p("/mod/notices"), map[string]any{"kind": "closure", "title": "Тропа на Иремель закрыта", "bbox": []float64{54.4, 58.7, 54.65, 59.05}, "starts_on": day(0)}, 201)
	mod.req("POST", p("/mod/notices"), map[string]any{"kind": "flood", "title": "Паводок", "bbox": []float64{54.9, 59.9, 55.2, 60.3}, "starts_on": day(-30), "ends_on": day(-20)}, 201)

	got := anna.req("GET", p("/routes/%s", rid), nil, 200)
	ns, _ := got["notices"].([]any)
	if len(ns) != 1 || ns[0].(map[string]any)["title"] != "Запрет посещения лесов" {
		t.Fatalf("route notices: %v", got["notices"])
	}
	if all := items(anna.req("GET", p("/notices"), nil, 200)); len(all) != 2 {
		t.Errorf("active notices: %d", len(all))
	}
	if all := items(mod.req("GET", p("/notices?all=1"), nil, 200)); len(all) != 3 {
		t.Errorf("all notices: %d", len(all))
	}

	// поход по маршруту: предупреждение на его даты; поход после окончания — без него
	start := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	h := anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": start, "route_id": rid}, 201)
	if got := anna.req("GET", p("/events/%v", h["id"]), nil, 200); len(got["notices"].([]any)) != 1 {
		t.Errorf("hike notices: %v", got["notices"])
	}
	later := time.Now().AddDate(0, 0, 20).UTC().Format(time.RFC3339)
	h2 := anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай позже", "starts_at": later, "route_id": rid}, 201)
	if got := anna.req("GET", p("/events/%v", h2["id"]), nil, 200); got["notices"] != nil {
		t.Errorf("notice outside the dates: %v", got["notices"])
	}

	// открытая страница маршрута
	resp, err := http.Get(ts.URL + "/trails/" + rid)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Противопожарный режим: Запрет посещения лесов") || !strings.Contains(string(b), `href="https://example.gov.ru/fire"`) {
		t.Error("trail page has no notice")
	}

	// изменить и удалить
	upd := map[string]any{}
	for k, v := range fire {
		upd[k] = v
	}
	upd["ends_on"] = day(-1)
	mod.req("PATCH", p("/mod/notices/%v", n["id"]), upd, 200)
	if got := anna.req("GET", p("/routes/%s", rid), nil, 200); got["notices"] != nil {
		t.Errorf("ended notice still shown: %v", got["notices"])
	}
	mod.req("DELETE", p("/mod/notices/%v", n["id"]), nil, 204)
	mod.req("DELETE", p("/mod/notices/%v", n["id"]), nil, 404)
}
