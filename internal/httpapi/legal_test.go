package httpapi

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestLegalPages(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	get := func(path string) (int, http.Header, string) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, string(b)
	}
	// реквизиты не заданы — видно, что их надо заполнить
	code, h, body := get("/legal/privacy")
	if code != 200 || !strings.Contains(body, "152-ФЗ") || !strings.Contains(body, "[оператор: LEGAL_OPERATOR]") {
		t.Fatalf("privacy: %d %.300s", code, body)
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") || strings.Contains(body, "<script") {
		t.Errorf("legal page must be static: %v", h)
	}
	srv.cfg.LegalOperator = `ИП Иванов <b>И.И.</b>`
	srv.cfg.LegalEmail = "pd@hike.example"
	_, _, body = get("/legal/consent")
	if !strings.Contains(body, "ИП Иванов &lt;b&gt;И.И.&lt;/b&gt;") || !strings.Contains(body, "pd@hike.example") {
		t.Errorf("operator details not substituted or not escaped: %.500s", body)
	}
	for _, d := range []string{"terms", "privacy", "consent"} {
		if code, _, _ := get("/legal/" + d); code != 200 {
			t.Errorf("%s: %d", d, code)
		}
	}
	if code, _, _ := get("/legal/nope"); code != 404 {
		t.Errorf("unknown doc: %d", code)
	}
	c := newClient(t, ts.URL)
	if code, out := c.do("GET", "/api/v1/legal", "", nil); code != 200 || len(out["documents"].([]any)) != 3 {
		t.Fatalf("legal list: %d %v", code, out)
	}
}

func TestConsents(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	// без отметок регистрации нет: каждая — отдельно
	c := newClient(t, ts.URL)
	code, out := c.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "teen@example.com", "username": "teen", "password": "password-teen", "accept_terms": true,
	})
	fields, _ := out["error"].(map[string]any)["fields"].(map[string]any)
	if code != 422 || fields["age_confirmed"] == nil || fields["pd_consent"] == nil || fields["accept_terms"] != nil {
		t.Fatalf("register without consents: %d %v", code, out)
	}

	anna := register(t, ts, "anna")
	st := anna.req("GET", p("/users/me/consents"), nil, 200)
	if st["required"] != false || len(st["log"].([]any)) != 4 {
		t.Fatalf("consents after register: %v", st)
	}
	if me := anna.req("GET", p("/users/me"), nil, 200); me["consent_required"] != false {
		t.Fatalf("me: %v", me)
	}

	// новая редакция документов: читать можно, менять — только после принятия
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE users SET legal_rev = 0 WHERE id = $1`, anna.ID); err != nil {
		t.Fatal(err)
	}
	if me := anna.req("GET", p("/users/me"), nil, 200); me["consent_required"] != true {
		t.Fatalf("me after new revision: %v", me)
	}
	anna.req("GET", p("/feed"), nil, 200)
	if out := anna.req("POST", p("/posts"), map[string]any{"text": "привет"}, 428); out["error"].(map[string]any)["code"] != "consent_required" {
		t.Fatalf("gate: %v", out)
	}
	anna.req("POST", p("/users/me/consents"), map[string]any{"age_confirmed": true, "accept_terms": true}, 422)
	anna.req("POST", p("/users/me/consents"), map[string]any{"age_confirmed": true, "accept_terms": true, "pd_consent": true}, 204)
	anna.req("POST", p("/posts"), map[string]any{"text": "привет"}, 201)
	if n := len(anna.req("GET", p("/users/me/consents"), nil, 200)["log"].([]any)); n != 8 {
		t.Fatalf("consent log: %d", n)
	}
}

func TestDeleteAccount(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	befriend(t, anna, bob)
	anna.req("POST", p("/posts"), map[string]any{"text": "Мой пост"}, 201)
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": "Привет, Боб"}, 201)
	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "track.gpx", []byte(hikeGPX()), nil)
	gpxPath, _ := srv.avStore.Path("routes", route["id"].(string), "route.gpx")
	if _, err := os.Stat(gpxPath); err != nil {
		t.Fatalf("route file: %v", err)
	}

	// сообщество с участниками переходит к ним, пустое — удаляется
	club := anna.req("POST", p("/communities"), map[string]any{"slug": "anna_club", "name": "Клуб"}, 201)
	clubID := int64(club["id"].(float64))
	bob.req("POST", p("/communities/%d/join", clubID), nil, 204)
	empty := anna.req("POST", p("/communities"), map[string]any{"slug": "anna_empty", "name": "Пусто"}, 201)
	anna.req("DELETE", p("/users/me"), map[string]any{"password": "wrong-password"}, 403)

	anna.req("DELETE", p("/users/me"), map[string]any{"password": "password-anna"}, 204)
	// вход и токены больше не работают
	anna.req("GET", p("/users/me"), nil, 401)
	if code, _ := newClient(t, ts.URL).do("POST", "/api/v1/auth/login", "", map[string]any{"email": "anna@example.com", "password": "password-anna"}); code != 401 {
		t.Fatalf("deleted account logs in: %d", code)
	}
	// профиль и записи пропали, файл трека стёрт
	bob.req("GET", p("/users/%d", anna.ID), nil, 404)
	if n := len(items(bob.req("GET", p("/feed"), nil, 200))); n != 0 {
		t.Fatalf("deleted user's posts in feed: %d", n)
	}
	if _, err := os.Stat(gpxPath); !os.IsNotExist(err) {
		t.Errorf("route file not deleted: %v", err)
	}
	// в базе не осталось почты и имени
	var email, first string
	if err := srv.store.Pool().QueryRow(t.Context(), `SELECT email, first_name FROM users WHERE id = $1`, anna.ID).Scan(&email, &first); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(email, "anna") || first == "anna" {
		t.Fatalf("not anonymized: %s %s", email, first)
	}
	// письмо Бобу осталось у него
	conv := items(bob.req("GET", p("/conversations"), nil, 200))
	if len(conv) != 1 {
		t.Fatalf("bob lost the conversation: %v", conv)
	}
	if got := bob.req("GET", p("/communities/%d", clubID), nil, 200); got["owner_id"].(float64) != float64(bob.ID) || got["my_role"] != "owner" {
		t.Fatalf("club not handed over: %v", got)
	}
	bob.req("GET", p("/communities/%d", int64(empty["id"].(float64))), nil, 404)
	// почту можно занять снова
	c := newClient(t, ts.URL)
	if code, out := c.do("POST", "/api/v1/auth/register", "", map[string]any{"email": "anna@example.com", "username": "anna",
		"password": "password-anna2", "age_confirmed": true, "accept_terms": true, "pd_consent": true}); code != 201 {
		t.Fatalf("re-register: %d %v", code, out)
	}
}
