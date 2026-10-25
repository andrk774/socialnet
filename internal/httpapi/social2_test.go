package httpapi

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestReactionsAndThreads(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	post := anna.req("POST", p("/posts"), map[string]any{"text": "пост"}, 201)
	pid := int64(post["id"].(float64))
	other := anna.req("POST", p("/posts"), map[string]any{"text": "другой"}, 201)

	// реакции: смена реакции не увеличивает счётчик
	bob.req("POST", p("/posts/%d/reactions", pid), map[string]any{"reaction": "love"}, 200)
	out := bob.req("POST", p("/posts/%d/reactions", pid), map[string]any{"reaction": "haha"}, 200)
	if out["likes_count"].(float64) != 1 {
		t.Fatalf("reaction change counted twice: %v", out)
	}
	eve.req("POST", p("/posts/%d/like", pid), nil, 200) // старый эндпоинт = like
	bob.req("POST", p("/posts/%d/reactions", pid), map[string]any{"reaction": "evil"}, 422)
	got := bob.req("GET", p("/posts/%d", pid), nil, 200)
	reacts := got["reactions"].(map[string]any)
	if got["likes_count"].(float64) != 2 || reacts["haha"].(float64) != 1 || reacts["like"].(float64) != 1 || got["my_reaction"] != "haha" {
		t.Fatalf("reactions: %v", got)
	}
	bob.req("DELETE", p("/posts/%d/reactions", pid), nil, 200)

	// ветка: ответ на ответ попадает в ту же ветку
	root := bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "корень"}, 201)
	rid := int64(root["id"].(float64))
	r1 := eve.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "ответ", "reply_to_id": rid}, 201)
	r2 := anna.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "ответ на ответ", "reply_to_id": int64(r1["id"].(float64))}, 201)
	if r2["parent_id"].(float64) != float64(rid) {
		t.Fatalf("nested reply not flattened: %v", r2)
	}
	// ответ на комментарий из другого поста — нельзя
	anna.req("POST", p("/posts/%d/comments", int64(other["id"].(float64))), map[string]any{"text": "x", "reply_to_id": rid}, 422)

	roots := items(bob.req("GET", p("/posts/%d/comments", pid), nil, 200))
	if len(roots) != 1 || roots[0].(map[string]any)["replies_count"].(float64) != 2 {
		t.Fatalf("roots: %v", roots)
	}
	replies := items(bob.req("GET", p("/comments/%d/replies", rid), nil, 200))
	if len(replies) != 2 || replies[1].(map[string]any)["reply_to"].(map[string]any)["username"] != "eve" {
		t.Fatalf("replies: %v", replies)
	}
	// Ева получила уведомление об ответе
	found := false
	for _, n := range items(eve.req("GET", p("/notifications"), nil, 200)) {
		found = found || n.(map[string]any)["type"] == "reply"
	}
	if !found {
		t.Fatal("no reply notification")
	}
	// удаление корня убирает ветку целиком
	bob.req("DELETE", p("/comments/%d", rid), nil, 204)
	if got := anna.req("GET", p("/posts/%d", pid), nil, 200); got["comments_count"].(float64) != 0 {
		t.Fatalf("comments_count after thread delete: %v", got["comments_count"])
	}
}

func TestGroupChats(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, carl, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "carl"), register(t, ts, "eve")
	eve.req("PATCH", p("/users/me/privacy"), map[string]any{"message_privacy": "friends"}, 200)
	bobWS := wsConnect(t, ts, bob)

	// Ева пишет только друзьям — её нельзя добавить
	anna.req("POST", p("/chats"), map[string]any{"title": "Поход", "member_ids": []int64{bob.ID, eve.ID}}, 403)
	chat := anna.req("POST", p("/chats"), map[string]any{"title": "Поход на Таганай", "member_ids": []int64{bob.ID, carl.ID}}, 201)
	cid := int64(chat["id"].(float64))
	if chat["members_count"].(float64) != 3 {
		t.Fatalf("chat: %v", chat)
	}
	if ev := waitEvent(t, bobWS, "message"); ev["kind"] != "service" {
		t.Fatalf("service message: %v", ev)
	}

	carl.req("POST", p("/conversations/%d/messages", cid), map[string]any{"body": "Беру палатку"}, 201)
	if ev := waitEvent(t, bobWS, "message"); ev["body"] != "Беру палатку" {
		t.Fatalf("group message: %v", ev)
	}
	carl.req("POST", p("/conversations/%d/messages", cid), map[string]any{"ciphertext": []byte("x"), "nonce": []byte("n")}, 422)
	eve.req("GET", p("/conversations/%d/messages", cid), nil, 404)
	eve.req("POST", p("/conversations/%d/messages", cid), map[string]any{"body": "впустите"}, 404)

	convs := items(bob.req("GET", p("/conversations"), nil, 200))
	if c := convs[0].(map[string]any); c["kind"] != "group" || c["title"] != "Поход на Таганай" || c["members_count"].(float64) != 3 {
		t.Fatalf("conversation list: %v", c)
	}

	// права: участник не добавляет и не переименовывает
	bob.req("POST", p("/conversations/%d/members", cid), map[string]any{"user_id": eve.ID}, 403)
	bob.req("PATCH", p("/conversations/%d", cid), map[string]any{"title": "Захват"}, 403)
	anna.req("PUT", p("/conversations/%d/members/%d/role", cid, bob.ID), map[string]any{"role": "admin"}, 204)
	bob.req("PATCH", p("/conversations/%d", cid), map[string]any{"title": "Таганай-2026"}, 204)
	bob.req("DELETE", p("/conversations/%d/members/%d", cid, anna.ID), nil, 403) // админ не исключает владельца
	bob.req("DELETE", p("/conversations/%d/members/%d", cid, carl.ID), nil, 204)
	carl.req("GET", p("/conversations/%d/messages", cid), nil, 404)

	// владелец уходит — им становится админ
	anna.req("DELETE", p("/conversations/%d/members/%d", cid, anna.ID), nil, 204)
	members := bob.req("GET", p("/conversations/%d/members", cid), nil, 200)["items"].([]any)
	if len(members) != 1 || members[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("ownership not transferred: %v", members)
	}
}

func TestMessageAttachments(t *testing.T) {
	needFFmpeg(t)
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")

	_, photo := upload(t, ts, anna, pngBytes(40, 40))
	_, evePhoto := upload(t, ts, eve, pngBytes(40, 40))
	_, track := uploadFile(t, ts, eve, "/api/v1/tracks", "t.mp3",
		genMedia(t, "t.mp3", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "libmp3lame"), nil)
	code, voice := uploadFile(t, ts, anna, "/api/v1/voice", "voice.ogg",
		genMedia(t, "v.ogg", "-f", "lavfi", "-i", "sine=frequency=200:duration=3", "-c:a", "libopus"), nil)
	if code != 201 || voice["duration_sec"].(float64) != 3 {
		t.Fatalf("voice upload: %d %v", code, voice)
	}

	msg := anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"attachments": []map[string]string{
		{"kind": "photo", "id": photo["id"].(string)},
		{"kind": "track", "id": track["id"].(string)}, // чужой трек — можно, музыкой делятся
		{"kind": "voice", "id": voice["id"].(string)},
	}}, 201)
	atts := msg["attachments"].([]any)
	if len(atts) != 3 || atts[2].(map[string]any)["duration_sec"].(float64) != 3 {
		t.Fatalf("attachments: %v", atts)
	}
	// чужое фото прикрепить нельзя
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"attachments": []map[string]string{
		{"kind": "photo", "id": evePhoto["id"].(string)}}}, 422)

	// голосовое слышат только участники диалога
	vurl := atts[2].(map[string]any)["url"].(string)
	get := func(u *tuser) int {
		req, _ := http.NewRequest("GET", ts.URL+vurl, nil)
		req.Header.Set("Authorization", "Bearer "+u.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if get(bob) != 200 || get(anna) != 200 || get(eve) != 404 {
		t.Fatalf("voice access: bob=%d anna=%d eve=%d", get(bob), get(anna), get(eve))
	}
	if resp, _ := http.Get(ts.URL + vurl); resp.StatusCode != 401 {
		t.Fatal("voice served without auth")
	}
}

func TestStories(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)
	_, photo := upload(t, ts, anna, pngBytes(30, 50))
	_, bobPhoto := upload(t, ts, bob, pngBytes(30, 50))

	anna.req("POST", p("/stories"), map[string]any{"photo_id": bobPhoto["id"]}, 422) // чужое фото
	anna.req("POST", p("/stories"), map[string]any{}, 422)
	st := anna.req("POST", p("/stories"), map[string]any{"photo_id": photo["id"], "caption": "Утро на Ильмене"}, 201)
	sid := int64(st["id"].(float64))

	feed := bob.req("GET", p("/stories/feed"), nil, 200)["items"].([]any)
	if len(feed) != 1 || feed[0].(map[string]any)["all_seen"] != false {
		t.Fatalf("feed: %v", feed)
	}
	bob.req("POST", p("/stories/%d/view", sid), nil, 204)
	bob.req("POST", p("/stories/%d/view", sid), nil, 204)
	if feed := bob.req("GET", p("/stories/feed"), nil, 200)["items"].([]any); feed[0].(map[string]any)["all_seen"] != true {
		t.Fatal("story not marked seen")
	}
	viewers := anna.req("GET", p("/stories/%d/viewers", sid), nil, 200)["items"].([]any)
	if len(viewers) != 1 {
		t.Fatalf("viewers: %v", viewers)
	}
	bob.req("GET", p("/stories/%d/viewers", sid), nil, 404) // список зрителей — только автору

	// закрытый профиль: чужие не видят истории
	anna.req("PATCH", p("/users/me/privacy"), map[string]any{"profile_visibility": "friends"}, 200)
	if n := len(eve.req("GET", p("/users/%d/stories", anna.ID), nil, 200)["items"].([]any)); n != 0 {
		t.Fatal("stranger sees stories of a closed profile")
	}
	eve.req("POST", p("/stories/%d/view", sid), nil, 404)

	// через 24 часа история исчезает
	srv.store.Pool().Exec(t.Context(), `UPDATE stories SET expires_at = now() - interval '1 minute'`)
	if n := len(bob.req("GET", p("/stories/feed"), nil, 200)["items"].([]any)); n != 0 {
		t.Fatal("expired story still visible")
	}
	bob.req("DELETE", p("/stories/%d", sid), nil, 404)
	anna.req("DELETE", p("/stories/%d", sid), nil, 204)
}

func TestClosedCommunityAndEvents(t *testing.T) {
	ts := setup(t, 1000)
	owner, editor, outsider := register(t, ts, "owner"), register(t, ts, "editor"), register(t, ts, "outsider")

	c := owner.req("POST", p("/communities"), map[string]any{"slug": "secret_club", "name": "Клуб", "visibility": "closed"}, 201)
	cid := int64(c["id"].(float64))
	owner.req("POST", p("/communities"), map[string]any{"slug": "x_club", "name": "X", "visibility": "secret"}, 422)
	post := owner.req("POST", p("/posts"), map[string]any{"text": "только для своих", "community_id": cid}, 201)
	pid := int64(post["id"].(float64))

	// посторонний не видит постов и участников, вступает только по заявке
	outsider.req("GET", p("/posts/%d", pid), nil, 404)
	if n := len(items(outsider.req("GET", p("/communities/%d/posts", cid), nil, 200))); n != 0 {
		t.Fatal("outsider sees closed community wall")
	}
	outsider.req("GET", p("/communities/%d/members", cid), nil, 403)
	if out := outsider.req("POST", p("/communities/%d/join", cid), nil, 202); out["status"] != "requested" {
		t.Fatalf("join closed: %v", out)
	}
	editor.req("POST", p("/communities/%d/join", cid), nil, 202)
	outsider.req("GET", p("/communities/%d/requests", cid), nil, 403)
	if n := len(items(owner.req("GET", p("/communities/%d/requests", cid), nil, 200))); n != 2 {
		t.Fatalf("requests: %d", n)
	}
	owner.req("POST", p("/communities/%d/requests/%d/approve", cid, editor.ID), nil, 204)
	owner.req("POST", p("/communities/%d/requests/%d/approve", cid, editor.ID), nil, 404)
	editor.req("GET", p("/posts/%d", pid), nil, 200)
	owner.req("PUT", p("/communities/%d/members/%d/role", cid, editor.ID), map[string]any{"role": "editor"}, 200)

	// мероприятие закрытого сообщества
	start := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(50 * time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	editor.req("POST", p("/events"), map[string]any{"title": "Сбор", "starts_at": start, "ends_at": past, "community_id": cid}, 422)
	editor.req("POST", p("/events"), map[string]any{"title": "Сбор", "starts_at": past}, 422)
	outsider.req("POST", p("/events"), map[string]any{"title": "Чужое", "starts_at": start, "community_id": cid}, 403)
	ev := editor.req("POST", p("/events"), map[string]any{"title": "Встреча клуба", "place": "Миасс, Ильменский",
		"starts_at": start, "ends_at": end, "community_id": cid}, 201)
	eid := int64(ev["id"].(float64))
	if ev["going_count"].(float64) != 1 || ev["my_status"] != "going" {
		t.Fatalf("event: %v", ev)
	}
	outsider.req("GET", p("/events/%d", eid), nil, 404)
	owner.req("POST", p("/events/%d/rsvp", eid), map[string]any{"status": "maybe"}, 204)
	if got := owner.req("GET", p("/events/%d", eid), nil, 200); got["maybe_count"].(float64) != 1 {
		t.Fatalf("rsvp: %v", got)
	}
	if n := len(items(owner.req("GET", p("/events/%d/attendees?status=going", eid), nil, 200))); n != 1 {
		t.Fatalf("attendees: %d", n)
	}
	if n := len(owner.req("GET", p("/events"), nil, 200)["items"].([]any)); n != 1 {
		t.Fatalf("upcoming: %d", n)
	}
	owner.req("PATCH", p("/events/%d", eid), map[string]any{"place": "Таганай"}, 200) // админ сообщества
	// личное мероприятие: менять может только организатор
	mine := outsider.req("POST", p("/events"), map[string]any{"title": "Мой день рождения", "starts_at": start}, 201)
	owner.req("PATCH", p("/events/%d", int64(mine["id"].(float64))), map[string]any{"title": "Захват"}, 403)
	owner.req("DELETE", p("/events/%d/rsvp", eid), nil, 204)
	editor.req("DELETE", p("/events/%d", eid), nil, 204)
}
