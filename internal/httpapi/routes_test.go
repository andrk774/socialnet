package httpapi

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"socialnet/internal/gpx"
)

// hikeGPX: «дом» в Миассе → 3 км на север с подъёмом → обратно домой.
func hikeGPX() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><gpx version="1.1" creator="Garmin eTrex 32x" xmlns="http://www.topografix.com/GPX/1/1">`)
	b.WriteString(`<metadata><author><name>Иван Петров</name><email id="ivan" domain="mail.ru"/></author></metadata>`)
	b.WriteString(`<wpt lat="55.050100" lon="60.100000"><name>Дом</name></wpt><wpt lat="55.070000" lon="60.100000"><ele>750</ele><name>Вершина</name></wpt>`)
	b.WriteString(`<trk><name>Таганай, Двуглавая сопка</name><trkseg>`)
	t0 := time.Date(2026, 7, 4, 6, 0, 0, 0, time.UTC)
	n := 0
	add := func(lat, ele float64) {
		fmt.Fprintf(&b, `<trkpt lat="%.6f" lon="60.100000"><ele>%.1f</ele><time>%s</time></trkpt>`,
			lat, ele, t0.Add(time.Duration(n)*10*time.Second).Format(time.RFC3339))
		n++
	}
	for i := 0; i <= 300; i++ { // 3 км вверх
		add(55.05+float64(i)*10/111195.0, 400+float64(i))
	}
	for i := 299; i >= 0; i-- { // обратно
		add(55.05+float64(i)*10/111195.0, 400+float64(i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	return b.String()
}

func TestRoutes(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, carol, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	befriend(t, anna, bob)
	home := [2]float64{55.05, 60.1}

	// отказ: не GPX, XXE, неверная настройка приватности
	for _, bad := range []string{`<html></html>`, `<?xml version="1.0"?><!DOCTYPE gpx [<!ENTITY x SYSTEM "file:///etc/passwd">]><gpx>&x;</gpx>`, `<gpx></gpx>`} {
		if code, out := uploadFile(t, ts, anna, "/api/v1/routes", "x.gpx", []byte(bad), nil); code != 422 {
			t.Errorf("bad gpx accepted: %d %v", code, out)
		}
	}
	if code, _ := uploadFile(t, ts, anna, "/api/v1/routes", "x.gpx", []byte(hikeGPX()), map[string]string{"hidden_ends_m": "50"}); code != 422 {
		t.Errorf("hidden_ends_m=50 accepted: %d", code)
	}
	// трек короче скрываемых концов
	short := `<gpx><trk><trkseg><trkpt lat="55" lon="60"/><trkpt lat="55.001" lon="60"/></trkseg></trk></gpx>`
	if code, _ := uploadFile(t, ts, anna, "/api/v1/routes", "s.gpx", []byte(short), nil); code != 422 {
		t.Errorf("short route with hidden ends accepted: %d", code)
	}

	code, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	if code != 201 {
		t.Fatalf("upload: %d %v", code, route)
	}
	rid := route["id"].(string)
	// честная статистика по полному треку
	if d := route["distance_m"].(float64); math.Abs(d-5990) > 20 {
		t.Errorf("distance %v", d)
	}
	if g := route["gain_m"].(float64); g < 290 || g > 310 {
		t.Errorf("gain %v", g)
	}
	if route["title"] != "Таганай, Двуглавая сопка" || route["hidden_ends_m"].(float64) != 200 || route["duration_sec"].(float64) != 6000 {
		t.Errorf("route: %v", route)
	}
	if _, ok := route["started_at"]; ok {
		t.Error("date of the hike leaked")
	}
	// на карте нет точек ближе 200 м к дому, путевая точка «Дом» убрана
	for _, seg := range route["detail"].([]any) {
		for _, p := range seg.([]any) {
			ll := p.([]any)
			if d := gpx.Distance(ll[0].(float64), ll[1].(float64), home[0], home[1]); d < 200 {
				t.Fatalf("map point %.0f m from home", d)
			}
		}
	}
	if wps := route["waypoints"].([]any); len(wps) != 1 || wps[0].(map[string]any)["name"] != "Вершина" {
		t.Errorf("waypoints: %v", wps)
	}
	if len(route["profile"].([]any)) < 50 || len(route["preview"].([]any)) > 160 {
		t.Errorf("profile %d preview %d", len(route["profile"].([]any)), len(route["preview"].([]any)))
	}

	// скачанный GPX — чистый
	c, h, body := download(t, ts, anna, route["gpx_url"].(string))
	if c != 200 || h.Get("Content-Type") != "application/gpx+xml" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("gpx download: %d %v", c, h)
	}
	for _, leak := range []string{"<time>", "Иван", "mail.ru", "Garmin", "Дом"} {
		if strings.Contains(string(body), leak) {
			t.Errorf("downloaded GPX leaks %q", leak)
		}
	}
	again, err := gpx.Parse(strings.NewReader(string(body)))
	if err != nil || again.Name != "Таганай, Двуглавая сопка" {
		t.Fatalf("downloaded GPX does not parse: %v", err)
	}

	// без записи маршрут виден только владельцу
	bob.req("GET", p("/routes/%s", rid), nil, 404)
	if c, _, _ := download(t, ts, bob, route["gpx_url"].(string)); c != 404 {
		t.Errorf("stranger downloaded: %d", c)
	}
	carol.req("POST", p("/posts"), map[string]any{"route_ids": []string{rid}}, 422) // чужой маршрут
	post := anna.req("POST", p("/posts"), map[string]any{"text": "Сходили на Двуглавую", "route_ids": []string{rid}, "audience": "friends"}, 201)
	routes := post["routes"].([]any)
	if len(routes) != 1 || routes[0].(map[string]any)["detail"] != nil || routes[0].(map[string]any)["gpx_url"] == "" {
		t.Fatalf("post routes (detail must not be in the feed): %v", routes)
	}
	bob.req("GET", p("/routes/%s", rid), nil, 200) // друг видит запись — видит и маршрут
	carol.req("GET", p("/routes/%s", rid), nil, 404)
	if c, _, _ := download(t, ts, bob, route["gpx_url"].(string)); c != 200 {
		t.Errorf("friend download: %d", c)
	}
	// маршрут без текста — тоже запись
	anna.req("POST", p("/posts"), map[string]any{"route_ids": []string{rid}, "audience": "only_me"}, 201)

	// список и переименование
	if l := items(anna.req("GET", p("/routes"), nil, 200)); len(l) != 1 || l[0].(map[string]any)["detail"] != nil {
		t.Errorf("my routes: %v", l)
	}
	bob.req("PATCH", p("/routes/%s", rid), map[string]any{"title": "моё"}, 404)
	anna.req("PATCH", p("/routes/%s", rid), map[string]any{"title": "Двуглавая, июль"}, 200)

	// жалоба и снятие: перезалить тот же файл нельзя
	carol.req("POST", p("/reports"), map[string]any{"target_type": "route", "target_uuid": rid, "reason": "спам"}, 404)
	bob.req("POST", p("/reports"), map[string]any{"target_type": "route", "target_uuid": rid, "reason": "спам"}, 201)
	mod.req("POST", p("/mod/routes/%s/hide", rid), nil, 204)
	bob.req("GET", p("/routes/%s", rid), nil, 404)
	if r := bob.req("GET", p("/posts/%d", idOf(post)), nil, 200)["routes"].([]any); len(r) != 0 {
		t.Errorf("hidden route still in post: %v", r)
	}
	if code, _ := uploadFile(t, ts, anna, "/api/v1/routes", "again.gpx", []byte(hikeGPX()), nil); code != 422 {
		t.Errorf("re-upload of removed route: %d", code)
	}
	mod.req("POST", p("/mod/routes/%s/unhide", rid), nil, 204)

	// без скрытия концов — маршрут целиком
	code, full := uploadFile(t, ts, bob, "/api/v1/routes", "full.gpx", []byte(hikeGPX()), map[string]string{"hidden_ends_m": "0", "title": "Полный"})
	if code != 201 || full["title"] != "Полный" {
		t.Fatalf("full upload: %d %v", code, full)
	}
	first := full["detail"].([]any)[0].([]any)[0].([]any)
	if d := gpx.Distance(first[0].(float64), first[1].(float64), home[0], home[1]); d > 1 {
		t.Errorf("hidden_ends_m=0 must keep the start: %.0f m", d)
	}

	// удаление
	anna.req("DELETE", p("/routes/%s", rid), nil, 204)
	bob.req("GET", p("/routes/%s", rid), nil, 404)
	anna.req("GET", p("/routes/not-a-uuid"), nil, 404)
}
