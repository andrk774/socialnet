package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func postIDs(m map[string]any) []int64 {
	var out []int64
	for _, it := range items(m) {
		out = append(out, int64(it.(map[string]any)["id"].(float64)))
	}
	return out
}

func idOf(m map[string]any) int64 { return int64(m["id"].(float64)) }

// ---------- аудитория поста ----------

func TestPostAudience(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, carol, dave := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "dave")
	befriend(t, anna, bob)
	befriend(t, anna, dave)
	carol.req("POST", p("/users/%d/friend", anna.ID), nil, 200) // carol — только подписчица

	pub := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "всем #audtest"}, 201))
	fr := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "друзьям #audtest", "audience": "friends"}, 201))
	me := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "себе @bob #audtest", "audience": "only_me"}, 201))
	ls := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "избранным #audtest", "audience": "list",
		"audience_ids": []int64{bob.ID, bob.ID}}, 201)) // повтор id не ошибка

	want := map[*tuser][]int64{
		anna:  {ls, me, fr, pub},
		bob:   {ls, fr, pub},
		dave:  {fr, pub},
		carol: {pub},
	}
	for u, ids := range want {
		for _, path := range []string{p("/users/%d/posts", anna.ID), p("/hashtags/audtest/posts")} {
			if got := postIDs(u.req("GET", path, nil, 200)); !slices.Equal(got, ids) {
				t.Errorf("%s %s: got %v, want %v", u.Username, path, got, ids)
			}
		}
		for _, id := range []int64{pub, fr, me, ls} {
			code := 404
			if slices.Contains(ids, id) {
				code = 200
			}
			u.req("GET", p("/posts/%d", id), nil, code)
			u.req("POST", p("/posts/%d/comments", id), map[string]any{"text": "hi"}, map[bool]int{true: 201, false: 404}[code == 200])
		}
	}
	// лента друга
	if got := postIDs(bob.req("GET", p("/feed"), nil, 200)); slices.Contains(got, me) || !slices.Contains(got, ls) {
		t.Errorf("bob feed: %v", got)
	}
	// упоминание в посте «только мне» не уведомляет — bob его не видит
	for _, n := range items(bob.req("GET", p("/notifications"), nil, 200)) {
		if n.(map[string]any)["type"] == "mention" {
			t.Error("bob got a mention notification for an only_me post")
		}
	}

	// в список можно выбрать только друзей
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "audience": "list", "audience_ids": []int64{carol.ID}}, 422)
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "audience": "list"}, 422)
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "audience": "friends", "audience_ids": []int64{bob.ID}}, 422)
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "audience": "nobody"}, 422)

	// аудиторию видит и меняет только автор
	a := anna.req("GET", p("/posts/%d/audience", ls), nil, 200)
	if a["audience"] != "list" || len(a["user_ids"].([]any)) != 1 {
		t.Errorf("audience: %v", a)
	}
	bob.req("GET", p("/posts/%d/audience", ls), nil, 403)
	bob.req("PATCH", p("/posts/%d", ls), map[string]any{"audience": "everyone"}, 403)
	anna.req("PATCH", p("/posts/%d", ls), map[string]any{"audience": "list", "audience_ids": []int64{dave.ID}}, 200)
	bob.req("GET", p("/posts/%d", ls), nil, 404)
	dave.req("GET", p("/posts/%d", ls), nil, 200)
	anna.req("PATCH", p("/posts/%d", me), map[string]any{"audience": "everyone"}, 200)
	carol.req("GET", p("/posts/%d", me), nil, 200)
	upd := anna.req("PATCH", p("/posts/%d", me), map[string]any{"text": "новый текст"}, 200)
	if upd["audience"] != "everyone" || upd["text"] != "новый текст" {
		t.Errorf("text edit changed audience: %v", upd)
	}

	// посты сообщества — всегда для аудитории сообщества
	cid := idOf(anna.req("POST", p("/communities"), map[string]any{"slug": "aud_club", "name": "Клуб"}, 201))
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "community_id": cid, "audience": "friends"}, 422)

	// удаление из друзей убирает доступ к посту «для друзей»
	dave.req("DELETE", p("/users/%d/friend", anna.ID), nil, 200)
	dave.req("GET", p("/posts/%d", fr), nil, 404)
}

// ---------- опросы ----------

func TestPolls(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna, bob, carol := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "carol")

	bad := []map[string]any{
		{"question": "", "options": []string{"a", "b"}},
		{"question": "Q", "options": []string{"a"}},
		{"question": "Q", "options": []string{"a", "A "}},
		{"question": "Q", "options": []string{"a", ""}},
		{"question": "Q", "options": strings.Split("1 2 3 4 5 6 7 8 9 10 11", " ")},
		{"question": "Q", "options": []string{"a", "b"}, "closes_at": time.Now().Add(-time.Hour)},
	}
	for _, poll := range bad {
		anna.req("POST", p("/posts"), map[string]any{"poll": poll}, 422)
	}

	post := anna.req("POST", p("/posts"), map[string]any{"poll": map[string]any{
		"question": "Куда пойдём?", "options": []string{"Тургояк", "Ильменский", "Таганай"}}}, 201)
	pid := idOf(post)
	poll := post["poll"].(map[string]any)
	opts := poll["options"].([]any)
	opt := func(i int) int64 { return int64(opts[i].(map[string]any)["id"].(float64)) }

	// до голосования счётчики скрыты (автор видит)
	got := bob.req("GET", p("/posts/%d", pid), nil, 200)["poll"].(map[string]any)
	if got["results_hidden"] != true || got["options"].([]any)[0].(map[string]any)["votes"] != nil {
		t.Errorf("results must be hidden before voting: %v", got)
	}
	bob.req("GET", p("/posts/%d/poll/options/%d/voters", pid, opt(0)), nil, 403)

	bob.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{opt(0), opt(1)}}, 422) // один вариант
	res := bob.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{opt(0)}}, 200)
	if res["voters_count"].(float64) != 1 || res["results_hidden"] != false {
		t.Errorf("after vote: %v", res)
	}
	bob.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{opt(1)}}, 409)
	bob.req("DELETE", p("/posts/%d/poll/votes", pid), nil, 200)
	bob.req("DELETE", p("/posts/%d/poll/votes", pid), nil, 409)
	bob.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{opt(2)}}, 200)

	// вариант из чужого опроса
	other := anna.req("POST", p("/posts"), map[string]any{"poll": map[string]any{
		"question": "Другой", "options": []string{"да", "нет"}, "multiple": true, "anonymous": true}}, 201)
	oid := idOf(other)
	foreign := int64(other["poll"].(map[string]any)["options"].([]any)[0].(map[string]any)["id"].(float64))
	carol.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{foreign}}, 422)
	// пост без опроса
	plain := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "без опроса"}, 201))
	carol.req("POST", p("/posts/%d/poll/votes", plain), map[string]any{"option_ids": []int64{opt(0)}}, 404)

	// кто голосовал — видно в открытом опросе, не видно в анонимном
	carol.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{opt(2)}}, 200)
	vs := items(carol.req("GET", p("/posts/%d/poll/options/%d/voters", pid, opt(2)), nil, 200))
	if len(vs) != 2 {
		t.Errorf("voters: %v", vs)
	}
	opts2 := other["poll"].(map[string]any)["options"].([]any)
	o2 := []int64{int64(opts2[0].(map[string]any)["id"].(float64)), int64(opts2[1].(map[string]any)["id"].(float64))}
	bob.req("POST", p("/posts/%d/poll/votes", oid), map[string]any{"option_ids": o2}, 200) // мультивыбор
	bob.req("GET", p("/posts/%d/poll/options/%d/voters", oid, o2[0]), nil, 403)

	// одновременные голоса: счётчики сходятся
	var users []*tuser
	for i := range 12 {
		users = append(users, register(t, ts, fmt.Sprintf("voter%d", i)))
	}
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := u.do("POST", p("/posts/%d/poll/votes", pid), u.Token, map[string]any{"option_ids": []int64{opt(i % 3)}})
			if code != 200 {
				t.Errorf("concurrent vote: %d", code)
			}
		}()
	}
	wg.Wait()
	final := anna.req("GET", p("/posts/%d", pid), nil, 200)["poll"].(map[string]any)
	sum := 0.0
	for _, o := range final["options"].([]any) {
		sum += o.(map[string]any)["votes"].(float64)
	}
	if final["voters_count"].(float64) != 14 || sum != 14 {
		t.Errorf("counts drifted: voters=%v sum=%v", final["voters_count"], sum)
	}

	// закрытый опрос: голосовать нельзя, результаты видны всем
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE polls SET closes_at = now() - interval '1 second' WHERE post_id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	stranger := register(t, ts, "stranger")
	stranger.req("POST", p("/posts/%d/poll/votes", pid), map[string]any{"option_ids": []int64{opt(0)}}, 409)
	carol.req("DELETE", p("/posts/%d/poll/votes", pid), nil, 409)
	closed := stranger.req("GET", p("/posts/%d", pid), nil, 200)["poll"].(map[string]any)
	if closed["closed"] != true || closed["results_hidden"] != false {
		t.Errorf("closed poll: %v", closed)
	}
}

// ---------- закладки ----------

func TestBookmarks(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	p1 := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "раз"}, 201))
	p2 := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "два"}, 201))
	hidden := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "секрет", "audience": "only_me"}, 201))

	bob.req("PUT", p("/bookmarks/post/%d", p1), nil, 204)
	bob.req("PUT", p("/bookmarks/post/%d", p2), nil, 204)
	bob.req("PUT", p("/bookmarks/post/%d", p2), nil, 204) // повтор — без ошибки
	bob.req("PUT", p("/bookmarks/post/%d", hidden), nil, 404)
	bob.req("PUT", p("/bookmarks/post/abc"), nil, 404)
	bob.req("PUT", p("/bookmarks/video/00000000-0000-0000-0000-000000000000"), nil, 404)
	bob.req("PUT", p("/bookmarks/user/%d", anna.ID), nil, 404)
	bob.req("GET", p("/bookmarks?kind=user"), nil, 422)

	got := bob.req("GET", p("/posts/%d", p1), nil, 200)
	if got["bookmarked"] != true {
		t.Error("bookmarked flag not set")
	}
	if anna.req("GET", p("/posts/%d", p1), nil, 200)["bookmarked"] != false {
		t.Error("bookmark leaked to another user")
	}
	list := items(bob.req("GET", p("/bookmarks?kind=post"), nil, 200))
	if len(list) != 2 || int64(list[0].(map[string]any)["post"].(map[string]any)["id"].(float64)) != p2 {
		t.Fatalf("bookmarks: %v", list)
	}
	// пост стал недоступен — пропадает из закладок, но убрать закладку можно
	anna.req("PATCH", p("/posts/%d", p2), map[string]any{"audience": "only_me"}, 200)
	if list := items(bob.req("GET", p("/bookmarks"), nil, 200)); len(list) != 1 {
		t.Errorf("hidden post still in bookmarks: %v", list)
	}
	bob.req("DELETE", p("/bookmarks/post/%d", p2), nil, 204)
	bob.req("DELETE", p("/bookmarks/post/%d", p1), nil, 204)
	if list := items(bob.req("GET", p("/bookmarks"), nil, 200)); len(list) != 0 {
		t.Errorf("after delete: %v", list)
	}
}

// ---------- отложенные публикации ----------

func TestScheduledPosts(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna, bob, ed := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "editor")
	ctx := t.Context()

	soon := time.Now().Add(time.Hour)
	anna.req("POST", p("/posts"), map[string]any{"text": "рано", "publish_at": time.Now().Add(5 * time.Second)}, 422)
	anna.req("POST", p("/posts"), map[string]any{"text": "поздно", "publish_at": time.Now().Add(400 * 24 * time.Hour)}, 422)
	anna.req("POST", p("/posts"), map[string]any{"poll": map[string]any{"question": "Q", "options": []string{"a", "b"},
		"closes_at": soon.Add(-time.Minute)}, "publish_at": soon}, 422) // опрос закрылся бы до публикации

	sp := anna.req("POST", p("/posts"), map[string]any{"text": "утренний пост #sched", "publish_at": soon,
		"poll": map[string]any{"question": "Кофе?", "options": []string{"да", "нет"}}}, 202)
	sid := idOf(sp)
	if sp["status"] != "pending" {
		t.Fatalf("scheduled: %v", sp)
	}
	if n := len(items(anna.req("GET", p("/users/%d/posts", anna.ID), nil, 200))); n != 0 {
		t.Fatalf("scheduled post visible before time: %d", n)
	}
	if l := items(anna.req("GET", p("/scheduled-posts"), nil, 200)); len(l) != 1 {
		t.Fatalf("list: %v", l)
	}
	bob.req("GET", p("/scheduled-posts"), nil, 200)
	bob.req("PATCH", p("/scheduled-posts/%d", sid), map[string]any{"publish_at": soon.Add(time.Hour)}, 404)
	bob.req("DELETE", p("/scheduled-posts/%d", sid), nil, 404)
	anna.req("PATCH", p("/scheduled-posts/%d", sid), map[string]any{"publish_at": soon.Add(time.Hour)}, 200)

	// время пришло — публикуем; повторный проход не дублирует
	if _, err := srv.store.Pool().Exec(ctx, `UPDATE scheduled_posts SET publish_at = now() WHERE id = $1`, sid); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 3 { // как несколько экземпляров API
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := srv.PublishDue(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	wall := items(bob.req("GET", p("/users/%d/posts", anna.ID), nil, 200))
	if len(wall) != 1 || wall[0].(map[string]any)["poll"] == nil {
		t.Fatalf("published wall: %v", wall)
	}
	if l := items(anna.req("GET", p("/scheduled-posts"), nil, 200)); len(l) != 0 {
		t.Errorf("published post still listed: %v", l)
	}
	anna.req("DELETE", p("/scheduled-posts/%d", sid), nil, 409)

	// отмена
	sid2 := idOf(anna.req("POST", p("/posts"), map[string]any{"text": "передумаю", "publish_at": soon}, 202))
	anna.req("DELETE", p("/scheduled-posts/%d", sid2), nil, 204)

	// сообщество: редактор планирует, его разжаловали — пост не выходит
	cid := idOf(anna.req("POST", p("/communities"), map[string]any{"slug": "sched_club", "name": "Клуб"}, 201))
	ed.req("POST", p("/communities/%d/join", cid), nil, 204)
	anna.req("PUT", p("/communities/%d/members/%d/role", cid, ed.ID), map[string]any{"role": "editor"}, 200)
	bob.req("POST", p("/posts"), map[string]any{"text": "x", "community_id": cid, "publish_at": soon}, 403)
	csid := idOf(ed.req("POST", p("/posts"), map[string]any{"text": "новости", "community_id": cid, "publish_at": soon}, 202))
	if l := items(anna.req("GET", p("/scheduled-posts?community_id=%d", cid), nil, 200)); len(l) != 1 {
		t.Errorf("community list for owner: %v", l)
	}
	bob.req("GET", p("/scheduled-posts?community_id=%d", cid), nil, 403)
	anna.req("PUT", p("/communities/%d/members/%d/role", cid, ed.ID), map[string]any{"role": "member"}, 200)
	if _, err := srv.store.Pool().Exec(ctx, `UPDATE scheduled_posts SET publish_at = now() WHERE id = $1`, csid); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.PublishDue(ctx); err != nil || n != 0 {
		t.Fatalf("PublishDue: %d %v", n, err)
	}
	l := items(ed.req("GET", p("/scheduled-posts"), nil, 200))
	if len(l) != 0 { // постов сообщества нет в личном списке
		t.Errorf("personal list: %v", l)
	}
	cl := items(anna.req("GET", p("/scheduled-posts?community_id=%d", cid), nil, 200))
	if len(cl) != 1 || cl[0].(map[string]any)["status"] != "failed" {
		t.Errorf("demoted editor's post must fail: %v", cl)
	}
	if n := len(items(bob.req("GET", p("/communities/%d/posts", cid), nil, 200))); n != 0 {
		t.Errorf("community wall: %d posts", n)
	}
}

// ---------- документы ----------

func download(t *testing.T, ts *httptest.Server, u *tuser, url string) (int, http.Header, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+url, nil)
	req.Header.Set("Authorization", "Bearer "+u.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func TestDocuments(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, carol, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	befriend(t, anna, bob)

	pdf := []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"evil.pdf", []byte("<html><script>alert(1)</script></html>")}, // сигнатура не совпала
		{"run.exe", []byte("MZ\x90\x00")},
		{"page.html", []byte("<html></html>")},
		{"img.svg", []byte("<svg/>")},
		{"bin.txt", []byte("abc\x00def")},
		{"noext", pdf},
	} {
		if code, out := uploadFile(t, ts, anna, "/api/v1/documents", c.name, c.data, nil); code != 415 {
			t.Errorf("%s: %d %v", c.name, code, out)
		}
	}
	code, doc := uploadFile(t, ts, anna, "/api/v1/documents", `..\..\Отчёт "Q3".pdf`, pdf, nil)
	if code != 201 || doc["ext"] != "pdf" || doc["title"] != `Отчёт "Q3"` {
		t.Fatalf("upload: %d %v", code, doc)
	}
	did := doc["id"].(string)
	code, txt := uploadFile(t, ts, anna, "/api/v1/documents", "notes.txt", []byte("Привет, мир"), map[string]string{"title": "Заметки"})
	if code != 201 || txt["title"] != "Заметки" {
		t.Fatalf("txt: %d %v", code, txt)
	}

	// скачать может владелец; заголовки не дают браузеру открыть файл у нас
	c, h, body := download(t, ts, anna, doc["url"].(string))
	if c != 200 || string(body) != string(pdf) {
		t.Fatalf("download: %d", c)
	}
	if h.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") ||
		h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("headers: %v", h)
	}
	if c, _, _ := download(t, ts, bob, doc["url"].(string)); c != 404 {
		t.Errorf("stranger downloaded a private document: %d", c)
	}

	// в посте для друзей — скачает друг, но не чужой
	carol.req("POST", p("/posts"), map[string]any{"document_ids": []string{did}}, 422) // не свой документ
	post := anna.req("POST", p("/posts"), map[string]any{"document_ids": []string{did}, "audience": "friends"}, 201)
	if docs := post["documents"].([]any); len(docs) != 1 {
		t.Fatalf("post documents: %v", post)
	}
	if c, _, _ := download(t, ts, bob, doc["url"].(string)); c != 200 {
		t.Errorf("friend download: %d", c)
	}
	if c, _, _ := download(t, ts, carol, doc["url"].(string)); c != 404 {
		t.Errorf("non-friend download: %d", c)
	}

	// в сообщении — скачает собеседник
	carol.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "x", "attachments": []map[string]any{{"kind": "document", "id": txt["id"]}}}, 422)
	msg := anna.req("POST", p("/users/%d/messages", carol.ID), map[string]any{"body": "держи",
		"attachments": []map[string]any{{"kind": "document", "id": txt["id"]}}}, 201)
	att := msg["attachments"].([]any)[0].(map[string]any)
	if att["kind"] != "document" || att["ext"] != "txt" {
		t.Errorf("attachment: %v", att)
	}
	if c, _, b := download(t, ts, carol, txt["url"].(string)); c != 200 || string(b) != "Привет, мир" {
		t.Errorf("message download: %d", c)
	}
	if c, _, _ := download(t, ts, bob, txt["url"].(string)); c != 404 {
		t.Errorf("outsider message download: %d", c)
	}

	// список, переименование, чужое не трогается
	if l := items(anna.req("GET", p("/documents"), nil, 200)); len(l) != 2 {
		t.Errorf("my documents: %v", l)
	}
	bob.req("PATCH", p("/documents/%s", did), map[string]any{"title": "моё"}, 404)
	bob.req("DELETE", p("/documents/%s", did), nil, 404)
	anna.req("PATCH", p("/documents/%s", did), map[string]any{"title": "Итоги"}, 200)

	// жалоба и снятие модератором — файл в блок-листе, перезалить нельзя
	carol.req("POST", p("/reports"), map[string]any{"target_type": "document", "target_uuid": did, "reason": "спам"}, 404) // не видит
	bob.req("POST", p("/reports"), map[string]any{"target_type": "document", "target_uuid": did, "reason": "спам"}, 201)
	bob.req("POST", p("/mod/documents/%s/hide", did), nil, 403)
	mod.req("POST", p("/mod/documents/%s/hide", did), nil, 204)
	if c, _, _ := download(t, ts, anna, doc["url"].(string)); c != 404 {
		t.Errorf("hidden document downloadable: %d", c)
	}
	if docs := bob.req("GET", p("/posts/%d", idOf(post)), nil, 200)["documents"].([]any); len(docs) != 0 {
		t.Errorf("hidden document still in post: %v", docs)
	}
	if code, _ := uploadFile(t, ts, anna, "/api/v1/documents", "again.pdf", pdf, nil); code != 422 {
		t.Errorf("re-upload of removed file: %d", code)
	}
	mod.req("POST", p("/mod/documents/%s/unhide", did), nil, 204)

	// удаление владельцем
	anna.req("DELETE", p("/documents/%s", did), nil, 204)
	if c, _, _ := download(t, ts, bob, doc["url"].(string)); c != 404 {
		t.Errorf("deleted document downloadable: %d", c)
	}
	anna.req("GET", p("/documents/not-a-uuid/download"), nil, 404)
}

func TestDetectDocument(t *testing.T) {
	cases := []struct {
		name string
		head string
		ok   bool
	}{
		{"a.PDF", "%PDF-1.7", true},
		{"a.docx", "PK\x03\x04", true},
		{"a.docx", "%PDF-", false},
		{"a.doc", "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1", true},
		{"a.csv", "a,b\n1,2", true},
		{"a.txt", "\xff\xfe\x00", false},
		{"a.txt", "ok \xd0", true}, // обрезанный посреди символа UTF-8
		{"a.js", "alert(1)", false},
		{"a.pdf.html", "%PDF-", false},
	}
	for _, c := range cases {
		if _, ok := detectDocument(c.name, []byte(c.head)); ok != c.ok {
			t.Errorf("%s %q: got %v", c.name, c.head, ok)
		}
	}
	if got := contentDisposition(`a"b\c;d.pdf`); !strings.Contains(got, `filename="a_b_c_d.pdf"`) {
		t.Errorf("content-disposition: %s", got)
	}
}

// ---------- админка ----------

func TestAdminPanel(t *testing.T) {
	ts, srv, box := setupFull(t, 1000)
	admin, anna, mod := register(t, ts, "admin"), register(t, ts, "anna"), register(t, ts, "moder")
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE users SET role = 'admin' WHERE id = $1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")

	for _, path := range []string{"/admin/stats", "/admin/users", fmt.Sprintf("/admin/users/%d", anna.ID)} {
		anna.req("GET", p("%s", path), nil, 403)
		mod.req("GET", p("%s", path), nil, 403)
	}
	st := admin.req("GET", p("/admin/stats"), nil, 200)
	if st["users"].(float64) != 3 || st["users_new_24h"].(float64) != 3 {
		t.Errorf("stats: %v", st)
	}
	if l := items(admin.req("GET", p("/admin/users?q=ANNA"), nil, 200)); len(l) != 1 {
		t.Errorf("search by username: %v", l)
	}
	if l := items(admin.req("GET", p("/admin/users?q=anna@example"), nil, 200)); len(l) != 1 {
		t.Errorf("search by email: %v", l)
	}
	if l := items(admin.req("GET", p("/admin/users?q=%d", anna.ID), nil, 200)); len(l) != 1 {
		t.Errorf("search by id: %v", l)
	}
	if l := items(admin.req("GET", p("/admin/users?q=%%25"), nil, 200)); len(l) != 0 {
		t.Errorf("LIKE wildcard not escaped: %v", l)
	}
	if l := items(admin.req("GET", p("/admin/users?filter=staff"), nil, 200)); len(l) != 2 {
		t.Errorf("staff filter: %v", l)
	}
	admin.req("GET", p("/admin/users?filter=everyone"), nil, 422)

	// в карточке нет ни пароля, ни его хэша, ни секретов
	card := admin.req("GET", p("/admin/users/%d", anna.ID), nil, 200)
	for k := range card {
		for _, bad := range []string{"password", "hash", "secret", "totp", "backup", "token"} {
			if strings.Contains(strings.ToLower(k), bad) {
				t.Errorf("admin card leaks %q", k)
			}
		}
	}
	if card["email"] != "anna@example.com" || card["active_sessions"].(float64) != 1 {
		t.Errorf("card: %v", card)
	}
	admin.req("GET", p("/admin/users/999999"), nil, 404)

	// сброс пароля: письмо уходит владельцу почты, админ ссылку не получает
	res := admin.req("POST", p("/admin/users/%d/password-reset", anna.ID), map[string]any{"revoke_sessions": true}, 202)
	if res["sent_to"] != "a***@example.com" || strings.Contains(fmt.Sprint(res), "token") {
		t.Errorf("reset response: %v", res)
	}
	mail := waitMail(t, box, "anna@example.com", "Сброс пароля")
	if mailCount(box, "admin@example.com", "Сброс пароля") != 0 {
		t.Error("reset link sent to admin")
	}
	anna.req("GET", p("/users/me"), nil, 401) // сеансы завершены
	tok := tokenInMail.FindStringSubmatch(mail.Body)[1]
	c := newClient(t, ts.URL)
	if code, out := c.do("POST", "/api/v1/auth/password/reset", "", map[string]any{"token": tok, "password": "new-password-123"}); code != 204 {
		t.Fatalf("reset: %d %v", code, out)
	}
	if _, _, code := login(t, ts, "anna@example.com", "new-password-123", ""); code != 200 {
		t.Errorf("login with new password: %d", code)
	}
	events := admin.req("GET", p("/admin/users/%d", anna.ID), nil, 200)["recent_security_events"].([]any)
	found := false
	for _, e := range events {
		ev := e.(map[string]any)
		if ev["type"] == "admin_password_reset" {
			found = true
			if ev["ip"] != nil {
				t.Errorf("admin IP leaked into user's security log: %v", ev)
			}
		}
	}
	if !found {
		t.Error("no admin_password_reset event")
	}

	// «выйти отовсюду»
	admin.req("POST", p("/admin/users/%d/revoke-sessions", mod.ID), nil, 204)
	mod.req("GET", p("/users/me"), nil, 401)
	admin.req("POST", p("/admin/users/%d/revoke-sessions", admin.ID), nil, 400)

	// всё записано в журнал аудита
	log := items(admin.req("GET", p("/admin/audit"), nil, 200))
	var actions []string
	for _, e := range log {
		actions = append(actions, e.(map[string]any)["action"].(string))
	}
	joined := strings.Join(actions, " ")
	if !strings.Contains(joined, "password-reset") || !strings.Contains(joined, "revoke-sessions") {
		t.Errorf("audit: %v", actions)
	}
}
