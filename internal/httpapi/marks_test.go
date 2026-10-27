package httpapi

import (
	"testing"
)

func TestRouteMarks(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, carol, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	befriend(t, anna, bob)
	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	rid := route["id"].(string)
	// маршрут в записи для друзей: Боб видит, Кэрол — нет
	anna.req("POST", p("/posts"), map[string]any{"text": "Таганай", "route_ids": []string{rid}, "audience": "friends"}, 201)

	near := map[string]any{"kind": "spring", "lat": 55.06, "lon": 60.1005, "title": "Родник у тропы", "note": "Вода чистая, течёт и в августе"}
	carol.req("POST", p("/routes/%s/marks", rid), near, 404)
	for name, bad := range map[string]map[string]any{
		"kind":    {"kind": "pub"},
		"far":     {"lat": 55.2},
		"nan lat": {"lat": 91},
		"title":   {"title": " "},
	} {
		body := map[string]any{}
		for k, v := range near {
			body[k] = v
		}
		for k, v := range bad {
			body[k] = v
		}
		if code, out := bob.do("POST", p("/routes/%s/marks", rid), bob.Token, body); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}
	// фото — только своё
	_, annaPhoto := upload(t, ts, anna, pngBytes(40, 30))
	withAnnaPhoto := map[string]any{"kind": "view", "lat": 55.07, "lon": 60.1, "title": "Вид на хребет", "photo_id": annaPhoto["id"]}
	bob.req("POST", p("/routes/%s/marks", rid), withAnnaPhoto, 422)
	_, bobPhoto := upload(t, ts, bob, pngBytes(40, 30))
	withAnnaPhoto["photo_id"] = bobPhoto["id"]
	view := bob.req("POST", p("/routes/%s/marks", rid), withAnnaPhoto, 201)
	if view["thumb_url"] == nil || view["verified_on"] == "" || view["author"].(map[string]any)["username"] != "bob" {
		t.Fatalf("mark: %v", view)
	}
	spring := bob.req("POST", p("/routes/%s/marks", rid), near, 201)
	sid := int64(spring["id"].(float64))

	// точки приходят вместе с маршрутом
	got := anna.req("GET", p("/routes/%s", rid), nil, 200)
	if marks, _ := got["marks"].([]any); len(marks) != 2 {
		t.Fatalf("route marks: %v", got["marks"])
	}
	// «актуально»
	chk := anna.req("POST", p("/routes/%s/marks/%d/check", rid, sid), nil, 200)
	anna.req("POST", p("/routes/%s/marks/%d/check", rid, sid), nil, 200)
	if chk["checks"].(float64) != 1 || chk["my_check"] != true {
		t.Fatalf("check: %v", chk)
	}
	carol.req("POST", p("/routes/%s/marks/%d/check", rid, sid), nil, 404)

	// жалоба и снятие модератором
	carol.req("POST", p("/reports"), map[string]any{"target_type": "route_point", "target_id": sid, "reason": "нет тут родника"}, 404)
	anna.req("POST", p("/reports"), map[string]any{"target_type": "route_point", "target_id": sid, "reason": "нет тут родника"}, 201)
	mod.req("POST", p("/mod/route-points/%d/hide", sid), nil, 204)
	if marks, _ := anna.req("GET", p("/routes/%s", rid), nil, 200)["marks"].([]any); len(marks) != 1 {
		t.Fatalf("hidden mark still listed: %d", len(marks))
	}
	// удаляет автор или владелец маршрута
	vid := int64(view["id"].(float64))
	_, carolRoute := uploadFile(t, ts, carol, "/api/v1/routes", "c.gpx", []byte(hikeGPX()), nil)
	bob.req("DELETE", p("/routes/%s/marks/%d", carolRoute["id"], vid), nil, 404) // чужой маршрут: не видит
	anna.req("DELETE", p("/routes/%s/marks/%d", rid, vid), nil, 204)
}
