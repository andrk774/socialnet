package httpapi

import (
	"net/http"
	"testing"
)

func TestFriendsFlow(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")

	// заявка → у Боба входящая, у Анны исходящая
	out := anna.req("POST", p("/users/%d/friend", bob.ID), nil, 200)
	if out["status"] != "request_sent" {
		t.Fatalf("status: %v", out)
	}
	if n := len(items(bob.req("GET", p("/friends/requests"), nil, 200))); n != 1 {
		t.Fatalf("incoming: %d", n)
	}
	if n := len(items(anna.req("GET", p("/friends/requests?direction=outgoing"), nil, 200))); n != 1 {
		t.Fatalf("outgoing: %d", n)
	}
	// Боб получил уведомление
	notes := bob.req("GET", p("/notifications"), nil, 200)
	if notes["unread"].(float64) != 1 || items(notes)[0].(map[string]any)["type"] != "friend_request" {
		t.Fatalf("notifications: %v", notes)
	}

	// принятие = встречная заявка
	out = bob.req("POST", p("/users/%d/friend", anna.ID), nil, 200)
	if out["status"] != "friends" {
		t.Fatalf("status after accept: %v", out)
	}
	if n := len(items(anna.req("GET", p("/users/%d/friends", bob.ID), nil, 200))); n != 1 {
		t.Fatalf("friends: %d", n)
	}
	prof := anna.req("GET", p("/users/%d", bob.ID), nil, 200)
	if prof["relationship"].(map[string]any)["status"] != "friends" {
		t.Fatalf("profile rel: %v", prof)
	}

	// удаление из друзей: Боб остаётся подписчиком (входящая заявка у Анны)
	anna.req("DELETE", p("/users/%d/friend", bob.ID), nil, 200)
	if n := len(items(anna.req("GET", p("/friends/requests"), nil, 200))); n != 1 {
		t.Fatalf("bob should stay as follower")
	}
	anna.req("POST", p("/users/%d/friend/reject", bob.ID), nil, 204)
	if n := len(items(anna.req("GET", p("/friends/requests"), nil, 200))); n != 0 {
		t.Fatalf("reject failed")
	}

	anna.req("POST", p("/users/%d/friend", anna.ID), nil, 400) // сам себя
	anna.req("POST", p("/users/%d/friend", 999999), nil, 404)  // несуществующий
	anna.req("GET", p("/users/by-username/bob"), nil, 200)
	anna.req("GET", p("/users/by-username/no_such"), nil, 404)
}

func TestPrivacy(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)

	anna.req("PATCH", p("/users/me"), map[string]any{"city": "Миасс", "status_text": "привет"}, 200)
	anna.req("PATCH", p("/users/me/privacy"), map[string]any{
		"profile_visibility": "friends", "friends_visibility": "only_me", "message_privacy": "friends",
	}, 200)
	anna.req("PATCH", p("/users/me/privacy"), map[string]any{"profile_visibility": "public"}, 422)

	// друг видит профиль, чужой — только имя
	if v := bob.req("GET", p("/users/%d", anna.ID), nil, 200); v["closed"] != false || v["city"] != "Миасс" {
		t.Fatalf("friend view: %v", v)
	}
	v := eve.req("GET", p("/users/%d", anna.ID), nil, 200)
	if v["closed"] != true || v["city"] != nil {
		t.Fatalf("stranger view leaks data: %v", v)
	}
	if v["relationship"].(map[string]any)["can_write"] != false {
		t.Fatalf("stranger can_write should be false")
	}

	// стена и список друзей
	anna.req("POST", p("/posts"), map[string]any{"text": "только для друзей"}, 201)
	if n := len(items(bob.req("GET", p("/users/%d/posts", anna.ID), nil, 200))); n != 1 {
		t.Fatalf("friend wall: %d", n)
	}
	eve.req("GET", p("/users/%d/posts", anna.ID), nil, 403)
	bob.req("GET", p("/users/%d/friends", anna.ID), nil, 403) // only_me
	anna.req("GET", p("/users/%d/friends", anna.ID), nil, 200)

	// пост закрытого профиля нельзя получить напрямую по id (IDOR)
	post := anna.req("POST", p("/posts"), map[string]any{"text": "секрет"}, 201)
	pid := int64(post["id"].(float64))
	eve.req("GET", p("/posts/%d", pid), nil, 404)
	eve.req("POST", p("/posts/%d/like", pid), nil, 404)
	eve.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "хи"}, 404)
	bob.req("GET", p("/posts/%d", pid), nil, 200)

	// свой профиль показывает настройки
	me := anna.req("GET", p("/users/me"), nil, 200)
	if me["privacy"].(map[string]any)["profile_visibility"] != "friends" {
		t.Fatalf("self privacy: %v", me)
	}
}

func TestBlocking(t *testing.T) {
	ts := setup(t, 1000)
	anna, troll := register(t, ts, "anna"), register(t, ts, "troll")
	befriend(t, anna, troll)
	post := anna.req("POST", p("/posts"), map[string]any{"text": "пост"}, 201)
	pid := int64(post["id"].(float64))

	anna.req("POST", p("/users/%d/block", troll.ID), nil, 204)

	// блокировка разрывает дружбу и закрывает всё
	if n := len(items(anna.req("GET", p("/users/%d/friends", anna.ID), nil, 200))); n != 0 {
		t.Fatalf("friendship should be removed")
	}
	troll.req("POST", p("/users/%d/friend", anna.ID), nil, 403)
	troll.req("GET", p("/posts/%d", pid), nil, 404)
	troll.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "!"}, 404)
	if v := troll.req("GET", p("/users/%d", anna.ID), nil, 200); v["closed"] != true {
		t.Fatalf("blocked user sees profile: %v", v)
	}
	if n := len(items(anna.req("GET", p("/blocks"), nil, 200))); n != 1 {
		t.Fatalf("blocks list")
	}

	anna.req("DELETE", p("/users/%d/block", troll.ID), nil, 204)
	troll.req("GET", p("/posts/%d", pid), nil, 200)
}

func TestPostsLikesComments(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)

	post := anna.req("POST", p("/posts"), map[string]any{"text": "Утро в #Миасс, привет @bob и @nobody_here"}, 201)
	pid := int64(post["id"].(float64))
	anna.req("POST", p("/posts"), map[string]any{"text": "   "}, 422)
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "photo_ids": []string{"not-a-uuid"}}, 422)
	anna.req("POST", p("/posts"), map[string]any{"text": "x", "photo_ids": []string{"00000000-0000-0000-0000-000000000000"}}, 422)

	// упоминание пришло Бобу
	found := false
	for _, n := range items(bob.req("GET", p("/notifications"), nil, 200)) {
		if n.(map[string]any)["type"] == "mention" {
			found = true
		}
	}
	if !found {
		t.Fatal("mention notification missing")
	}

	// лайки: повторный лайк не увеличивает счётчик
	bob.req("POST", p("/posts/%d/like", pid), nil, 200)
	out := bob.req("POST", p("/posts/%d/like", pid), nil, 200)
	if out["likes_count"].(float64) != 1 {
		t.Fatalf("double like counted: %v", out)
	}
	eve.req("POST", p("/posts/%d/like", pid), nil, 200)
	eve.req("DELETE", p("/posts/%d/like", pid), nil, 200)
	eve.req("DELETE", p("/posts/%d/like", pid), nil, 200)
	if got := anna.req("GET", p("/posts/%d", pid), nil, 200); got["likes_count"].(float64) != 1 {
		t.Fatalf("likes_count: %v", got["likes_count"])
	}

	// комментарии и права на удаление
	c1 := bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "класс!"}, 201)
	c2 := eve.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "спам"}, 201)
	c1id, c2id := int64(c1["id"].(float64)), int64(c2["id"].(float64))
	eve.req("DELETE", p("/comments/%d", c1id), nil, 403)  // чужой комментарий
	anna.req("DELETE", p("/comments/%d", c2id), nil, 204) // автор поста может удалить любой
	if n := len(items(anna.req("GET", p("/posts/%d/comments", pid), nil, 200))); n != 1 {
		t.Fatalf("comments: %d", n)
	}
	if got := anna.req("GET", p("/posts/%d", pid), nil, 200); got["comments_count"].(float64) != 1 {
		t.Fatalf("comments_count: %v", got["comments_count"])
	}

	// хэштег без учёта регистра
	if n := len(items(eve.req("GET", p("/hashtags/миасс/posts"), nil, 200))); n != 1 {
		t.Fatalf("hashtag search: %d", n)
	}

	// редактировать может только автор; правка обновляет хэштеги
	bob.req("PATCH", p("/posts/%d", pid), map[string]any{"text": "взлом"}, 403)
	anna.req("PATCH", p("/posts/%d", pid), map[string]any{"text": "теперь #челябинск"}, 200)
	if n := len(items(eve.req("GET", p("/hashtags/миасс/posts"), nil, 200))); n != 0 {
		t.Fatalf("old hashtag should be gone")
	}

	// лента: Боб видит пост друга, Ева (не подписана) — нет
	if n := len(items(bob.req("GET", p("/feed"), nil, 200))); n != 1 {
		t.Fatalf("bob feed: %d", n)
	}
	if n := len(items(eve.req("GET", p("/feed"), nil, 200))); n != 0 {
		t.Fatalf("eve feed: %d", n)
	}

	// удаление
	bob.req("DELETE", p("/posts/%d", pid), nil, 403)
	anna.req("DELETE", p("/posts/%d", pid), nil, 204)
	bob.req("GET", p("/posts/%d", pid), nil, 404)
}

func TestFeedPagination(t *testing.T) {
	ts := setup(t, 1000)
	anna := register(t, ts, "anna")
	for i := 0; i < 5; i++ {
		anna.req("POST", p("/posts"), map[string]any{"text": "пост"}, 201)
	}
	page1 := anna.req("GET", p("/feed?limit=3"), nil, 200)
	if len(items(page1)) != 3 || page1["next_cursor"] == nil {
		t.Fatalf("page1: %v", page1)
	}
	cur := int64(page1["next_cursor"].(float64))
	page2 := anna.req("GET", p("/feed?limit=3&before=%d", cur), nil, 200)
	if len(items(page2)) != 2 || page2["next_cursor"] != nil {
		t.Fatalf("page2: %v", page2)
	}
	_ = http.StatusOK
}

func TestNotificationsRead(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	bob.req("POST", p("/users/%d/friend", anna.ID), nil, 200)
	eve.req("POST", p("/users/%d/friend", anna.ID), nil, 200)

	list := anna.req("GET", p("/notifications"), nil, 200)
	first := int64(items(list)[0].(map[string]any)["id"].(float64))

	// Ева не может пометить чужие уведомления
	eve.req("POST", p("/notifications/read"), map[string]any{"ids": []int64{first}}, 204)
	if anna.req("GET", p("/notifications"), nil, 200)["unread"].(float64) != 2 {
		t.Fatal("foreign user marked notification read")
	}
	anna.req("POST", p("/notifications/read"), map[string]any{"ids": []int64{first}}, 204)
	if anna.req("GET", p("/notifications"), nil, 200)["unread"].(float64) != 1 {
		t.Fatal("mark one")
	}
	anna.req("POST", p("/notifications/read"), map[string]any{}, 204)
	if anna.req("GET", p("/notifications"), nil, 200)["unread"].(float64) != 0 {
		t.Fatal("mark all")
	}
}
