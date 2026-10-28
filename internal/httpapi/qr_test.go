package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHikeQR(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.cfg.AppURL = "https://hike.example"
	srv.publicLimiter = newIPRateLimiter(100, 100)
	org, bob, carol := register(t, ts, "org"), register(t, ts, "bob"), register(t, ts, "carol")
	start := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	h := org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Иремель", "starts_at": start}, 201)
	hid := int64(h["id"].(float64))

	// картинка: SVG без скриптов, доступна тем, кто видит поход
	req, _ := http.NewRequest("GET", ts.URL+p("/events/%d/qr.svg", hid), nil)
	req.Header.Set("Authorization", "Bearer "+bob.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	svg, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" ||
		!strings.HasPrefix(string(svg), "<svg") || strings.Contains(string(svg), "script") ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("qr.svg: %d %v %.80s", resp.StatusCode, resp.Header, svg)
	}

	// скан: без входа, перенаправление с ?from=qr; повтор из того же браузера не считается
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	scan := func(ua string, cookie *http.Cookie) *http.Response {
		r, _ := http.NewRequest("GET", ts.URL+p("/qr/e/%d", hid)[len("/api/v1"):], nil)
		r.Header.Set("User-Agent", ua)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		resp, err := noFollow.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	phone := "Mozilla/5.0 (Linux; Android 14) Mobile"
	first := scan(phone, nil)
	if first.StatusCode != 302 || first.Header.Get("Location") != p("/events/%d?from=qr", hid)[len("/api/v1"):] {
		t.Fatalf("scan: %d %s", first.StatusCode, first.Header.Get("Location"))
	}
	var seen *http.Cookie
	for _, c := range first.Cookies() {
		if strings.HasPrefix(c.Name, "qr_e") {
			seen = c
		}
	}
	if seen == nil || !seen.HttpOnly {
		t.Fatalf("no dedupe cookie: %v", first.Cookies())
	}
	scan(phone, seen)                                          // тот же браузер — не считается
	scan("TelegramBot (like TwitterBot)", nil)                 // превью ссылки — не считается
	scan("Mozilla/5.0 (iPhone; CPU iPhone OS 18) Mobile", nil) // другой телефон

	// запись после скана считается; повторная — нет; без source — нет
	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going", "source": "qr"}, 200)
	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going", "source": "qr"}, 200)
	carol.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)

	st := org.req("GET", p("/events/%d/qr-stats", hid), nil, 200)
	if st["scans"].(float64) != 2 || st["joins"].(float64) != 1 || st["scans_week"].(float64) != 2 || len(st["days"].([]any)) != 1 {
		t.Fatalf("stats: %v", st)
	}
	bob.req("GET", p("/events/%d/qr-stats", hid), nil, 403)

	// несуществующий поход: перенаправление есть, счётчика нет
	r, _ := http.NewRequest("GET", ts.URL+"/qr/e/999999", nil)
	r.Header.Set("User-Agent", phone)
	if resp, err := noFollow.Do(r); err != nil || resp.StatusCode != 302 {
		t.Fatalf("missing event: %v %v", err, resp)
	}
	if r2, _ := http.Get(ts.URL + "/qr/e/abc"); r2.StatusCode != 404 {
		t.Errorf("bad id: %d", r2.StatusCode)
	}
}
