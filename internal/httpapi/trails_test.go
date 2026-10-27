package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPublicTrails(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	srv.publicLimiter = newIPRateLimiter(1000, 1000)
	srv.cfg.AppURL = "https://hike.example"
	anna, bob, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	get := func(path string) (int, http.Header, string) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, string(b)
	}
	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	rid := route["id"].(string)

	// закрытый маршрут без входа не виден, в каталоге его нет
	if code, _, _ := get("/trails/" + rid); code != 404 {
		t.Fatalf("private trail: %d", code)
	}
	bob.req("GET", p("/routes/%s", rid), nil, 404)
	bob.req("PATCH", p("/routes/%s", rid), map[string]any{"public": true}, 404) // не владелец

	upd := anna.req("PATCH", p("/routes/%s", rid), map[string]any{"public": true, "public_note": "Тропа от <b>Белого ключа</b>\nВода у родника"}, 200)
	if upd["public"] != true {
		t.Fatalf("public: %v", upd)
	}
	anna.req("POST", p("/routes/%s/marks", rid), map[string]any{"kind": "spring", "lat": 55.06, "lon": 60.1, "title": "Родник"}, 201)
	// открытый маршрут видят все вошедшие
	bob.req("GET", p("/routes/%s", rid), nil, 200)

	code, h, list := get("/trails")
	if code != 200 || !strings.Contains(list, "Таганай, Двуглавая сопка") || !strings.Contains(list, "/trails/"+rid) {
		t.Fatalf("list: %d %.400s", code, list)
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") || strings.Contains(list, "<script") {
		t.Errorf("public page must be static: %v", h)
	}
	code, _, page := get("/trails/" + rid)
	for _, want := range []string{"<h1>Таганай, Двуглавая сопка</h1>", "6 км", "↑ 300 м", "Родник", "&lt;b&gt;Белого ключа&lt;/b&gt;",
		`<link rel="canonical" href="https://hike.example/trails/` + rid + `">`, "/trails/" + rid + ".gpx", "class=\"route-profile\""} {
		if !strings.Contains(page, want) {
			t.Errorf("trail page lacks %q", want)
		}
	}
	if code != 200 || strings.Contains(page, "<b>Белого") {
		t.Fatalf("trail: %d", code)
	}
	code, h, gpxFile := get("/trails/" + rid + ".gpx")
	if code != 200 || !strings.Contains(h.Get("Content-Disposition"), "attachment") || strings.Contains(gpxFile, "<time>") || strings.Contains(gpxFile, "Иван") {
		t.Fatalf("public gpx: %d %v", code, h)
	}

	code, _, sm := get("/sitemap.xml")
	if code != 200 || !strings.Contains(sm, "<loc>https://hike.example/trails/"+rid+"</loc>") || !strings.Contains(sm, "/legal/privacy") {
		t.Fatalf("sitemap: %s", sm)
	}
	_, _, robots := get("/robots.txt")
	if !strings.Contains(robots, "Allow: /trails") || !strings.Contains(robots, "Disallow: /\n") || !strings.Contains(robots, "Sitemap: https://hike.example/sitemap.xml") {
		t.Fatalf("robots: %s", robots)
	}

	// снят модератором — пропадает отовсюду
	mod.req("POST", p("/mod/routes/%s/hide", rid), nil, 204)
	if code, _, _ := get("/trails/" + rid); code != 404 {
		t.Errorf("hidden trail still public: %d", code)
	}
	if _, _, sm := get("/sitemap.xml"); strings.Contains(sm, rid) {
		t.Error("hidden trail in sitemap")
	}
	if code, _, _ := get("/trails?page=abc"); code != 404 {
		t.Errorf("bad page: %d", code)
	}
}
