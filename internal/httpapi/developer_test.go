package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/safehttp"
)

// Каждый маршрут из таблицы прав существует и находится тем же шаблоном,
// иначе опечатка в таблице молча закрыла бы метод для приложений.
func TestScopeTableMatchesRouter(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	_ = ts
	param := regexp.MustCompile(`\{[a-z]+\}`)
	for route := range routeScope {
		method, pattern, _ := strings.Cut(route, " ")
		path := param.ReplaceAllString(pattern, "1")
		if got := srv.root.Find(chi.NewRouteContext(), method, path); got != pattern {
			t.Errorf("%s: router finds %q", route, got)
		}
	}
}

func pkce() (verifier, challenge string) {
	verifier = strings.Repeat("v", 20) + "-._~" + strings.Repeat("A", 30)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func postForm(t *testing.T, ts *httptest.Server, path string, vals url.Values, user, pass string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != "" {
		req.SetBasicAuth(url.QueryEscape(user), url.QueryEscape(pass))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// as — клиент с токеном приложения вместо обычного входа.
func as(u *tuser, token string) *tuser {
	return &tuser{testClient: u.testClient, ID: u.ID, Username: u.Username + "/app", Token: token}
}

func TestOAuthFlow(t *testing.T) {
	ts := setup(t, 1000)
	dev, anna := register(t, ts, "dev"), register(t, ts, "anna")
	const cb = "https://app.example/cb"

	for _, bad := range []string{"javascript:alert(1)", "http://evil.example/cb", cb + "#x", "https://u:p@app.example/cb", "data:text/html,x"} {
		dev.req("POST", p("/apps"), map[string]any{"name": "X", "redirect_uris": []string{bad}}, 422)
	}
	app := dev.req("POST", p("/apps"), map[string]any{"name": "Погода", "redirect_uris": []string{cb, "http://127.0.0.1:8765/cb", "com.example.weather:/cb"}}, 201)
	clientID := app["client_id"].(string)
	if app["client_secret"] != nil || app["confidential"] != false {
		t.Fatalf("public app: %v", app)
	}
	anna.req("GET", p("/apps/%d", int64(app["id"].(float64))), nil, 404) // чужое приложение

	verifier, challenge := pkce()
	q := func(redirect, scope string) string {
		return p("/oauth/authorize?") + url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect},
			"scope": {scope}, "state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}.Encode()
	}
	anna.req("GET", q("https://evil.example/cb", "wall"), nil, 400)
	anna.req("GET", q(cb, "wall root"), nil, 400)
	anna.req("GET", strings.Replace(q(cb, "wall"), "S256", "plain", 1), nil, 400)
	info := anna.req("GET", q(cb, "wall profile"), nil, 200)
	if info["app"].(map[string]any)["developer"] != "dev" || len(info["scopes"].([]any)) != 2 {
		t.Fatalf("consent info: %v", info)
	}

	authorize := func(scope string, approve bool) url.Values {
		t.Helper()
		out := anna.req("POST", p("/oauth/authorize"), map[string]any{"response_type": "code", "client_id": clientID,
			"redirect_uri": cb, "scope": scope, "state": "xyz", "code_challenge": challenge,
			"code_challenge_method": "S256", "approve": approve}, 200)
		u, _ := url.Parse(out["redirect_to"].(string))
		if !strings.HasPrefix(u.String(), cb+"?") || u.Query().Get("state") != "xyz" || u.Query().Get("iss") != "https://app.test" {
			t.Fatalf("redirect_to: %v", u)
		}
		return u.Query()
	}
	if denied := authorize("wall", false); denied.Get("error") != "access_denied" || denied.Get("code") != "" {
		t.Fatalf("deny: %v", denied)
	}
	exchange := func(code, redirect, verifier string) (int, map[string]any) {
		return postForm(t, ts, "/api/v1/oauth/token", url.Values{"grant_type": {"authorization_code"}, "code": {code},
			"redirect_uri": {redirect}, "client_id": {clientID}, "code_verifier": {verifier}}, "", "")
	}
	if code, out := exchange(authorize("wall", true).Get("code"), cb, strings.Repeat("x", 50)); code != 400 || out["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier: %d %v", code, out)
	}
	if code, _ := exchange(authorize("wall", true).Get("code"), "http://127.0.0.1:8765/cb", verifier); code != 400 {
		t.Fatal("redirect_uri mismatch accepted")
	}
	if code, out := postForm(t, ts, "/api/v1/oauth/token", url.Values{"grant_type": {"password"}}, "", ""); code != 400 || out["error"] != "unsupported_grant_type" {
		t.Fatalf("password grant: %d %v", code, out)
	}

	// повторное предъявление кода отзывает выданный по нему токен
	reused := authorize("wall", true).Get("code")
	code, tok := exchange(reused, cb, verifier)
	if code != 200 || !strings.HasPrefix(tok["access_token"].(string), "snt_") || tok["expires_in"].(float64) != 86400 {
		t.Fatalf("token: %d %v", code, tok)
	}
	leaked := as(anna, tok["access_token"].(string))
	leaked.req("GET", p("/feed"), nil, 200)
	if code, _ := exchange(reused, cb, verifier); code != 400 {
		t.Fatal("code accepted twice")
	}
	leaked.req("GET", p("/feed"), nil, 401)

	// права токена
	_, tok = exchange(authorize("wall profile", true).Get("code"), cb, verifier)
	appAnna := as(anna, tok["access_token"].(string))
	if me := appAnna.req("GET", p("/users/me"), nil, 200); me["username"] != "anna" {
		t.Fatalf("me: %v", me)
	}
	appAnna.req("POST", p("/posts"), map[string]any{"text": "из приложения"}, 201)
	if out := appAnna.req("GET", p("/notifications"), nil, 403); out["error"].(map[string]any)["code"] != "insufficient_scope" {
		t.Fatalf("scope: %v", out)
	}
	for _, rt := range [][2]string{{"GET", "/sessions"}, {"POST", "/users/me/password"}, {"GET", "/wallet"},
		{"PUT", "/users/me/keys"}, {"POST", "/apps"}, {"GET", "/users/me/authorized-apps"},
		{"PATCH", "/users/me/privacy"}, {"POST", "/users/me/2fa/setup"}, {"GET", "/security/events"}} {
		if out := appAnna.req(rt[0], p(rt[1]), map[string]any{}, 403); out["error"].(map[string]any)["code"] != "not_available" {
			t.Fatalf("%v: %v", rt, out)
		}
	}

	// список «приложения с доступом» и отзыв
	apps := items(anna.req("GET", p("/users/me/authorized-apps"), nil, 200))
	if len(apps) != 1 || apps[0].(map[string]any)["name"] != "Погода" {
		t.Fatalf("authorized apps: %v", apps)
	}
	anna.req("DELETE", p("/users/me/authorized-apps/%d", int64(app["id"].(float64))), nil, 204)
	appAnna.req("GET", p("/users/me"), nil, 401)

	// offline — без срока; смена пароля отзывает токены приложений
	_, tok = exchange(authorize("offline wall", true).Get("code"), cb, verifier)
	if _, has := tok["expires_in"]; has {
		t.Fatalf("offline token expires: %v", tok)
	}
	off := as(anna, tok["access_token"].(string))
	off.req("GET", p("/feed"), nil, 200)
	anna.req("POST", p("/users/me/password"), map[string]any{"current_password": "password-anna", "new_password": "new-password-anna"}, 204)
	off.req("GET", p("/feed"), nil, 401)
}

func TestOAuthConfidentialClient(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	dev, anna := register(t, ts, "dev"), register(t, ts, "anna")
	const cb = "https://srv.example/cb"
	app := dev.req("POST", p("/apps"), map[string]any{"name": "Сервер", "redirect_uris": []string{cb}, "confidential": true}, 201)
	other := dev.req("POST", p("/apps"), map[string]any{"name": "Другое", "redirect_uris": []string{cb}}, 201)
	clientID, secret := app["client_id"].(string), app["client_secret"].(string)
	verifier, challenge := pkce()
	newCode := func() string {
		out := anna.req("POST", p("/oauth/authorize"), map[string]any{"response_type": "code", "client_id": clientID,
			"redirect_uri": cb, "scope": "profile", "code_challenge": challenge, "code_challenge_method": "S256", "approve": true}, 200)
		u, _ := url.Parse(out["redirect_to"].(string))
		return u.Query().Get("code")
	}
	form := func(code string) url.Values {
		return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {cb}, "code_verifier": {verifier}}
	}
	if code, out := postForm(t, ts, "/api/v1/oauth/token", form(newCode()), clientID, "wrong"); code != 401 || out["error"] != "invalid_client" {
		t.Fatalf("wrong secret: %d %v", code, out)
	}
	// код выдан одному приложению — другое его не обменяет
	f := form(newCode())
	f.Set("client_id", other["client_id"].(string))
	if code, _ := postForm(t, ts, "/api/v1/oauth/token", f, "", ""); code != 400 {
		t.Fatal("code exchanged by another client")
	}
	code, tok := postForm(t, ts, "/api/v1/oauth/token", form(newCode()), clientID, secret)
	if code != 200 {
		t.Fatalf("basic auth: %d %v", code, tok)
	}
	// у админа токен приложения не даёт прав админа
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE users SET role = 'admin' WHERE id = $1`, anna.ID); err != nil {
		t.Fatal(err)
	}
	appAnna := as(anna, tok["access_token"].(string))
	appAnna.req("GET", p("/admin/vulnerability-reports"), nil, 403)
	appAnna.req("GET", p("/mod/reports"), nil, 403)

	// отзыв по RFC 7009, затем смена секрета
	if code, _ := postForm(t, ts, "/api/v1/oauth/revoke", url.Values{"token": {tok["access_token"].(string)}}, clientID, secret); code != 200 {
		t.Fatal("revoke")
	}
	appAnna.req("GET", p("/users/me"), nil, 401)
	rotated := dev.req("POST", p("/apps/%d/secret", int64(app["id"].(float64))), nil, 200)
	if code, _ := postForm(t, ts, "/api/v1/oauth/token", form(newCode()), clientID, secret); code != 401 {
		t.Fatal("old secret still works")
	}
	if code, _ := postForm(t, ts, "/api/v1/oauth/token", form(newCode()), clientID, rotated["client_secret"].(string)); code != 200 {
		t.Fatal("new secret rejected")
	}
	// истёкший токен
	_, tok = postForm(t, ts, "/api/v1/oauth/token", form(newCode()), clientID, rotated["client_secret"].(string))
	srv.store.Pool().Exec(t.Context(), `UPDATE api_tokens SET expires_at = now() - interval '1 second'`) //nolint:errcheck
	as(anna, tok["access_token"].(string)).req("GET", p("/users/me"), nil, 401)
}

type callbackRecorder struct {
	mu     sync.Mutex
	secret string
	code   string
	status int
	events []map[string]any
	badSig int
}

func (c *callbackRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	m := hmac.New(sha256.New, []byte(c.secret))
	m.Write([]byte(r.Header.Get("X-Socialnet-Timestamp") + "."))
	m.Write(body)
	if !hmac.Equal([]byte(r.Header.Get("X-Socialnet-Signature")), []byte("sha256="+hex.EncodeToString(m.Sum(nil)))) {
		c.badSig++
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var ev map[string]any
	_ = json.Unmarshal(body, &ev)
	if c.status != 0 {
		w.WriteHeader(c.status)
		return
	}
	if ev["type"] == "confirmation" {
		_, _ = io.WriteString(w, c.code)
		return
	}
	c.events = append(c.events, ev)
	_, _ = io.WriteString(w, "ok")
}

func TestBotsAndCallbackAPI(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	owner, anna, carl := register(t, ts, "owner"), register(t, ts, "anna"), register(t, ts, "carl")
	c := owner.req("POST", p("/communities"), map[string]any{"slug": "weather_bot", "name": "Погода в Миассе"}, 201)
	cid := int64(c["id"].(float64))

	// имена bot<id> зарезервированы
	newClient(t, ts.URL).do("POST", "/api/v1/auth/register", "", nil)
	if code, _ := newClient(t, ts.URL).do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "b@example.com", "username": "Bot1", "password": "password-123"}); code != 422 {
		t.Fatalf("reserved bot name: %d", code)
	}

	anna.req("POST", p("/communities/%d/bot/tokens", cid), map[string]any{"scopes": []string{"messages"}}, 403)
	owner.req("POST", p("/communities/%d/bot/tokens", cid), map[string]any{"scopes": []string{"friends"}}, 422)
	bt := owner.req("POST", p("/communities/%d/bot/tokens", cid), map[string]any{"name": "прод", "scopes": []string{"messages", "wall"}}, 201)
	botID := int64(bt["bot_user_id"].(float64))
	bot := &tuser{testClient: owner.testClient, ID: botID, Username: "bot", Token: bt["token"].(string)}
	if n := len(items(owner.req("GET", p("/communities/%d/bot/tokens", cid), nil, 200))); n != 1 {
		t.Fatalf("bot tokens: %d", n)
	}

	// стена: только своего сообщества
	bot.req("POST", p("/posts"), map[string]any{"text": "личный пост"}, 403)
	other := anna.req("POST", p("/communities"), map[string]any{"slug": "anna_club", "name": "Клуб Анны"}, 201)
	bot.req("POST", p("/posts"), map[string]any{"text": "чужая стена", "community_id": other["id"]}, 403)
	bot.req("POST", p("/posts"), map[string]any{"text": "Завтра +12, ясно", "community_id": cid}, 201)
	bot.req("GET", p("/notifications"), nil, 403)
	bot.req("POST", p("/chats"), map[string]any{"title": "спам", "member_ids": []int64{anna.ID}}, 403)

	// сообщения: бот не пишет первым
	bot.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "Купите!"}, 403)

	// Callback API
	rec := &callbackRecorder{}
	cbServer := httptest.NewServer(rec)
	defer cbServer.Close()
	srv.callbackInsecure = true
	srv.callbackClient = cbServer.Client()

	owner.req("PUT", p("/communities/%d/callback", cid), map[string]any{"url": "ftp://x", "events": []string{"message_new"}}, 422)
	owner.req("PUT", p("/communities/%d/callback", cid), map[string]any{"url": cbServer.URL, "events": []string{"bogus"}}, 422)
	set := owner.req("PUT", p("/communities/%d/callback", cid), map[string]any{"url": cbServer.URL,
		"events": []string{"message_new", "wall_post_new", "wall_reply_new", "group_join"}}, 200)
	rec.secret = set["secret"].(string)

	// до подтверждения события не копятся
	anna.req("POST", p("/users/%d/messages", botID), map[string]any{"body": "Какая погода?"}, 201)
	rec.code = "wrong"
	owner.req("POST", p("/communities/%d/callback/confirm", cid), nil, 422)
	rec.code = set["confirmation_code"].(string)
	owner.req("POST", p("/communities/%d/callback/confirm", cid), nil, 200)
	if n, _ := srv.DispatchCallbacks(t.Context(), 100); n != 0 {
		t.Fatalf("events queued before confirmation: %d", n)
	}

	reply := bot.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "Завтра +12"}, 201)
	if reply["sender_id"].(float64) != float64(botID) {
		t.Fatalf("bot reply: %v", reply)
	}
	anna.req("POST", p("/users/%d/messages", botID), map[string]any{"body": "Спасибо"}, 201)
	post := owner.req("POST", p("/posts"), map[string]any{"text": "Новости", "community_id": cid}, 201)
	carl.req("POST", p("/posts/%d/comments", int64(post["id"].(float64))), map[string]any{"text": "Класс"}, 201)
	carl.req("POST", p("/communities/%d/join", cid), nil, 204)
	carl.req("POST", p("/communities/%d/leave", cid), nil, 204) // group_leave не подписан
	if n, err := srv.DispatchCallbacks(t.Context(), 100); err != nil || n != 4 {
		t.Fatalf("dispatched %d, err %v", n, err)
	}
	var types []string
	for _, ev := range rec.events {
		types = append(types, ev["type"].(string))
		if ev["group_id"].(float64) != float64(cid) || ev["event_id"] == "" {
			t.Fatalf("event: %v", ev)
		}
	}
	if strings.Join(types, ",") != "message_new,wall_post_new,wall_reply_new,group_join" || rec.badSig != 0 {
		t.Fatalf("events: %v, bad signatures: %d", types, rec.badSig)
	}
	msg := rec.events[0]["object"].(map[string]any)["message"].(map[string]any)
	if msg["body"] != "Спасибо" {
		t.Fatalf("message_new: %v", msg)
	}

	// сбой сервера: повторы с паузой, потом статус failing
	rec.status = 500
	carl.req("POST", p("/communities/%d/join", cid), nil, 204)
	for i := 0; i < callbackMaxAttempts; i++ {
		srv.store.Pool().Exec(t.Context(), `UPDATE callback_deliveries SET run_after = now() WHERE status = 'queued'`) //nolint:errcheck
		if n, _ := srv.DispatchCallbacks(t.Context(), 100); n != 1 {
			t.Fatalf("attempt %d: dispatched %d", i+1, n)
		}
	}
	if cs := owner.req("GET", p("/communities/%d/callback", cid), nil, 200)["server"].(map[string]any); cs["status"] != "failing" {
		t.Fatalf("status after failures: %v", cs)
	}

	// SSRF: в боевом режиме только https и никаких внутренних адресов
	srv.callbackInsecure = false
	srv.callbackClient = safehttp.Client(2 * time.Second)
	owner.req("PUT", p("/communities/%d/callback", cid), map[string]any{"url": cbServer.URL, "events": []string{"message_new"}}, 422)
	internal := strings.Replace(cbServer.URL, "http://", "https://", 1)
	owner.req("PUT", p("/communities/%d/callback", cid), map[string]any{"url": internal, "events": []string{"message_new"},
		"secret": strings.Repeat("s", 20)}, 200)
	out := owner.req("POST", p("/communities/%d/callback/confirm", cid), nil, 422)
	if !strings.Contains(out["error"].(map[string]any)["message"].(string), "internal address") {
		t.Fatalf("ssrf: %v", out)
	}

	// отзыв токена бота
	owner.req("DELETE", p("/communities/%d/bot/tokens/%d", cid, int64(bt["id"].(float64))), nil, 204)
	bot.req("GET", p("/conversations"), nil, 401)
}
