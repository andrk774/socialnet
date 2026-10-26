package httpapi

import (
	"strings"
	"testing"
	"time"
)

func TestCompanionAds(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, eve, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }

	// Анна принимает сообщения только от друзей — отклики всё равно доходят
	anna.req("PATCH", p("/users/me/privacy"), map[string]any{"message_privacy": "friends"}, 200)

	for name, bad := range map[string]map[string]any{
		"no region":   {"region": " "},
		"past":        {"date_from": day(-5)},
		"bad date":    {"date_from": "07.11.2026"},
		"to < from":   {"date_to": day(1)},
		"too long":    {"date_to": day(90)},
		"difficulty":  {"difficulty": 0},
		"seats":       {"seats": 21},
		"bad route":   {"route_id": "x"},
		"other route": {"route_id": "00000000-0000-4000-8000-000000000000"},
	} {
		body := map[string]any{"region": "Таганай", "title": "Ищу попутчиков", "date_from": day(3), "date_to": day(4)}
		for k, v := range bad {
			body[k] = v
		}
		if code, out := anna.do("POST", p("/companions"), anna.Token, body); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}

	ad := anna.req("POST", p("/companions"), map[string]any{
		"region": "Таганай", "title": "Ищу 2 человек на Откликной гребень", "body": "Ночёвка у Белого ключа",
		"date_from": day(3), "date_to": day(4), "difficulty": 3, "seats": 2,
	}, 201)
	aid := int64(ad["id"].(float64))
	if ad["date_from"] != day(3) || ad["seats"].(float64) != 2 || ad["author"].(map[string]any)["username"] != "anna" {
		t.Fatalf("ad: %v", ad)
	}
	anna.req("POST", p("/companions"), map[string]any{"region": "Иремель", "title": "Зимний Иремель", "date_from": day(20)}, 201)

	// фильтры: район без учёта регистра, пересечение дат, сложность
	count := func(q string) int { return len(items(bob.req("GET", p("/companions%s", q), nil, 200))) }
	for q, want := range map[string]int{
		"": 2, "?region=таган": 1, "?region=%25": 0, "?from=" + day(10): 1, "?to=" + day(3): 1,
		"?from=" + day(4) + "&to=" + day(4): 1, "?max_difficulty=2": 1, "?max_difficulty=3": 2,
	} {
		if got := count(q); got != want {
			t.Errorf("filter %q: %d, want %d", q, got, want)
		}
	}
	bob.req("GET", p("/companions?from=завтра"), nil, 400)
	bob.req("GET", p("/companions?max_difficulty=7"), nil, 400)

	// отклик — личное сообщение автору, хотя Боб ей не друг
	bob.req("POST", p("/companions/%d/reply", aid), map[string]any{"text": " "}, 422)
	bob.req("POST", p("/companions/%d/reply", aid), map[string]any{"text": "Пойду, есть палатка на двоих"}, 201)
	anna.req("POST", p("/companions/%d/reply", aid), map[string]any{"text": "сам себе"}, 400)
	conv := items(anna.req("GET", p("/conversations"), nil, 200))
	if len(conv) != 1 {
		t.Fatalf("anna conversations: %v", conv)
	}
	last := conv[0].(map[string]any)["last_message"].(map[string]any)["body"].(string)
	if !strings.Contains(last, "Откликной гребень") || !strings.Contains(last, "есть палатка") {
		t.Fatalf("reply body: %q", last)
	}
	// Анна может ответить Бобу, хотя у неё «только друзья», — он написал первым
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": "Отлично, жду в субботу"}, 201)
	// а постороннему без отклика по-прежнему нельзя
	eve.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "привет"}, 403)

	// закрыто — откликнуться нельзя, из поиска пропадает; чужое не закрыть
	bob.req("PATCH", p("/companions/%d", aid), map[string]any{"closed": true}, 404)
	anna.req("PATCH", p("/companions/%d", aid), map[string]any{"closed": true}, 200)
	eve.req("POST", p("/companions/%d/reply", aid), map[string]any{"text": "можно?"}, 409)
	if got := count(""); got != 1 {
		t.Fatalf("closed still listed: %d", got)
	}
	if n := len(items(anna.req("GET", p("/companions/mine"), nil, 200))); n != 2 {
		t.Fatalf("mine: %d", n)
	}
	anna.req("PATCH", p("/companions/%d", aid), map[string]any{"closed": false}, 200)

	// блокировка: заблокированный не видит объявления и не откликается
	anna.req("POST", p("/users/%d/block", eve.ID), nil, 204)
	eve.req("GET", p("/companions/%d", aid), nil, 404)
	eve.req("POST", p("/companions/%d/reply", aid), map[string]any{"text": "ну пожалуйста"}, 404)
	if n := len(items(eve.req("GET", p("/companions"), nil, 200))); n != 0 {
		t.Fatalf("blocked user sees ads: %d", n)
	}

	// не больше 5 открытых объявлений
	for i := range 3 {
		anna.req("POST", p("/companions"), map[string]any{"region": "Урал", "title": "Ещё", "date_from": day(5 + i)}, 201)
	}
	anna.req("POST", p("/companions"), map[string]any{"region": "Урал", "title": "Шестое", "date_from": day(9)}, 422)

	// жалоба и снятие модератором
	bob.req("POST", p("/reports"), map[string]any{"target_type": "companion", "target_id": aid, "reason": "спам"}, 201)
	mod.req("POST", p("/mod/companions/%d/hide", aid), nil, 204)
	bob.req("GET", p("/companions/%d", aid), nil, 404)

	anna.req("DELETE", p("/companions/%d", aid), nil, 204)
	anna.req("DELETE", p("/companions/%d", aid), nil, 404)
}
