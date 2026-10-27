package httpapi

import (
	"testing"
	"time"
)

// Модератор видит в списке жалоб, что именно обжаловано (заголовок,
// фрагмент, автор, скрыто ли), для объявлений, статей, точек и маршрутов,
// и может скрыть и вернуть объект.
func TestReportTargets(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")

	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	rid := route["id"].(string)
	anna.req("PATCH", p("/routes/%s", rid), map[string]any{"public": true, "public_note": "Кольцо через Двуглавую"}, 200)
	mark := anna.req("POST", p("/routes/%s/marks", rid), map[string]any{"kind": "spring", "lat": 55.06, "lon": 60.1005, "title": "Родник у тропы", "note": "течёт всё лето"}, 201)
	ad := anna.req("POST", p("/companions"), map[string]any{"region": "Таганай", "title": "Ищу попутчиков на Откликной", "body": "две ночёвки",
		"date_from": time.Now().AddDate(0, 0, 3).Format("2006-01-02")}, 201)
	art := anna.req("POST", p("/articles"), map[string]any{"title": "Три дня на Таганае", "body": "Вышли из Златоуста рано утром", "status": "published"}, 201)

	report := func(body map[string]any) { bob.req("POST", p("/reports"), body, 201) }
	report(map[string]any{"target_type": "companion", "target_id": ad["id"], "reason": "спам"})
	report(map[string]any{"target_type": "article", "target_id": art["id"], "reason": "реклама"})
	report(map[string]any{"target_type": "route_point", "target_id": mark["id"], "reason": "нет родника"})
	report(map[string]any{"target_type": "route", "target_uuid": rid, "reason": "опасный маршрут"})

	byType := func(status string) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, it := range items(mod.req("GET", p("/mod/reports?status=%s", status), nil, 200)) {
			r := it.(map[string]any)
			out[r["target_type"].(string)] = r
		}
		return out
	}
	open := byType("open")
	for typ, want := range map[string][2]string{
		"companion":   {"Ищу попутчиков на Откликной", "Таганай: две ночёвки"},
		"article":     {"Три дня на Таганае", "Вышли из Златоуста рано утром"},
		"route_point": {"Родник у тропы", "течёт всё лето"},
		"route":       {route["title"].(string), "Кольцо через Двуглавую"},
	} {
		tg, _ := open[typ]["target"].(map[string]any)
		if tg == nil || tg["title"] != want[0] || tg["snippet"] != want[1] || tg["hidden"] != false || int64(tg["author_id"].(float64)) != anna.ID || tg["author"] != "anna" {
			t.Errorf("%s target: %v", typ, open[typ])
		}
	}
	if open["route_point"]["target"].(map[string]any)["ref"] != rid {
		t.Errorf("route point should link to its route: %v", open["route_point"]["target"])
	}

	// скрыть через решение по жалобе — в разобранных видно, что скрыто
	for _, typ := range []string{"companion", "article", "route_point", "route"} {
		mod.req("POST", p("/mod/reports/%v/resolve", open[typ]["id"]), map[string]any{"action": "hide"}, 204)
	}
	done := byType("resolved")
	for _, typ := range []string{"companion", "article", "route_point", "route"} {
		if tg, _ := done[typ]["target"].(map[string]any); tg == nil || tg["hidden"] != true {
			t.Errorf("%s not hidden: %v", typ, done[typ])
		}
	}
	bob.req("GET", p("/articles/%v", art["id"]), nil, 404)

	// вернуть
	mod.req("POST", p("/mod/articles/%v/unhide", art["id"]), nil, 204)
	mod.req("POST", p("/mod/routes/%s/unhide", rid), nil, 204)
	bob.req("GET", p("/articles/%v", art["id"]), nil, 200)
	if tg := byType("resolved")["article"]["target"].(map[string]any); tg["hidden"] != false {
		t.Errorf("article still hidden: %v", tg)
	}

	// удалённый объект — target нет, жалоба остаётся
	ad2 := anna.req("POST", p("/companions"), map[string]any{"region": "Иремель", "title": "Зимний Иремель",
		"date_from": time.Now().AddDate(0, 0, 9).Format("2006-01-02")}, 201)
	bob.req("POST", p("/reports"), map[string]any{"target_type": "companion", "target_id": ad2["id"], "reason": "ещё раз"}, 201)
	anna.req("DELETE", p("/companions/%v", ad2["id"]), nil, 204)
	if r := byType("open")["companion"]; r == nil || r["target"] != nil {
		t.Errorf("deleted target: %v", r)
	}
}
