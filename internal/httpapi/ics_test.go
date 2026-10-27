package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"socialnet/internal/store"
)

func TestEventICS(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.cfg.AppURL = "https://hike.example"
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	start := time.Date(2030, 7, 12, 3, 0, 0, 0, time.UTC)
	h := anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай; Откликной, гребень",
		"description": "Две ночёвки\nу Белого ключа", "starts_at": start.Format(time.RFC3339),
		"ends_at": start.Add(30 * time.Hour).Format(time.RFC3339), "meeting_point": "Златоуст, ж/д вокзал",
		"gear": "палатка, горелка"}, 201)
	id := int64(h["id"].(float64))

	get := func(c *tuser) (int, http.Header, string) {
		req, _ := http.NewRequest("GET", ts.URL+p("/events/%d/calendar.ics", id), nil)
		if c != nil {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, string(b)
	}
	if code, _, _ := get(nil); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	code, hdr, ics := get(bob)
	if code != 200 || !strings.HasPrefix(hdr.Get("Content-Type"), "text/calendar") || !strings.Contains(hdr.Get("Content-Disposition"), ".ics") {
		t.Fatalf("ics: %d %v", code, hdr)
	}
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "UID:event-", "@hike.example\r\n", "DTSTART:20300712T030000Z\r\n", "DTEND:20300713T090000Z\r\n",
		`SUMMARY:Таганай\; Откликной\, гребень`, `Две ночёвки\nу Белого ключа`, `Сбор: Златоуст\, ж/д вокзал`,
		"LOCATION:Златоуст\\, ж/д вокзал\r\n", "URL:https://hike.example/events/", "TRIGGER:-P1D", "END:VCALENDAR\r\n",
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("no %q in\n%s", want, ics)
		}
	}
	for _, l := range strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n") {
		if len(l) > 75 || !utf8.ValidString(l) {
			t.Errorf("bad folding: %q", l)
		}
	}
	if strings.Contains(strings.ReplaceAll(ics, "\r\n", ""), "\n") {
		t.Error("bare LF in ics")
	}

	// без конца — три часа; удалённое — 404
	e := &store.Event{ID: 7, Title: "Встреча", StartsAt: start}
	if s := eventICS(e, "http://x", start); !strings.Contains(s, "DTEND:20300712T060000Z") {
		t.Errorf("default end: %s", s)
	}
	anna.req("DELETE", p("/events/%d", id), nil, 204)
	if code, _, _ := get(bob); code != 404 {
		t.Errorf("deleted event calendar: %d", code)
	}
}
