package httpapi

import (
	"strings"
	"testing"
)

func TestArticles(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, eve, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")
	_, photo := upload(t, ts, anna, pngBytes(80, 60))
	_, bobPhoto := upload(t, ts, bob, pngBytes(80, 60))
	pid, bpid := photo["id"].(string), bobPhoto["id"].(string)
	_, route := uploadFile(t, ts, anna, "/api/v1/routes", "t.gpx", []byte(hikeGPX()), nil)
	rid := route["id"].(string)

	body := "## День первый\n\nВышли в **7 утра**. [Прогноз](https://open-meteo.com) обещал ясно.\n\n" +
		"![Вид с Двуглавой](media:" + pid + ")\n\n- вода\n- палатка\n\n<img src=x onerror=alert(1)> [x](javascript:alert(1))"

	// чужие фото и маршрут, пустой текст при публикации, неверный статус
	anna.req("POST", p("/articles"), map[string]any{"title": "Таганай", "body": "![](media:" + bpid + ")"}, 422)
	anna.req("POST", p("/articles"), map[string]any{"title": "Таганай", "body": "текст", "cover_media_id": bpid}, 422)
	bob.req("POST", p("/articles"), map[string]any{"title": "Чужой маршрут", "body": "текст", "route_id": rid}, 422)
	anna.req("POST", p("/articles"), map[string]any{"title": "Пусто", "body": " ", "status": "published"}, 422)
	anna.req("POST", p("/articles"), map[string]any{"title": "Таганай", "body": "x", "status": "secret"}, 422)
	anna.req("POST", p("/articles"), map[string]any{"title": " ", "body": "x"}, 422)

	a := anna.req("POST", p("/articles"), map[string]any{"title": "Три дня на Таганае", "body": body, "cover_media_id": pid, "route_id": rid}, 201)
	aid := int64(a["id"].(float64))
	if a["status"] != "draft" || a["cover_thumb_url"] == nil {
		t.Fatalf("article: %v", a)
	}
	blocks := a["blocks"].([]any)
	if len(blocks) != 5 || blocks[2].(map[string]any)["kind"] != "img" {
		t.Fatalf("blocks: %v", blocks)
	}
	last := blocks[4].(map[string]any)["spans"].([]any)
	if len(last) != 1 || !strings.Contains(last[0].(map[string]any)["text"].(string), "<img src=x") {
		t.Fatalf("html must stay text, javascript: links dropped: %v", last)
	}

	// черновик видит только автор
	bob.req("GET", p("/articles/%d", aid), nil, 404)
	if n := len(items(bob.req("GET", p("/articles"), nil, 200))); n != 0 {
		t.Fatalf("draft listed: %d", n)
	}
	bob.req("PATCH", p("/articles/%d", aid), map[string]any{"status": "published"}, 404)
	anna.req("PATCH", p("/articles/%d", aid), map[string]any{"status": "published"}, 200)

	got := bob.req("GET", p("/articles/%d", aid), nil, 200)
	if got["published_at"] == nil || got["author"].(map[string]any)["username"] != "anna" {
		t.Fatalf("published: %v", got)
	}
	list := items(bob.req("GET", p("/articles"), nil, 200))
	if len(list) != 1 || list[0].(map[string]any)["body"] != nil || !strings.HasPrefix(list[0].(map[string]any)["excerpt"].(string), "День первый") {
		t.Fatalf("list: %v", list)
	}
	// маршрут статьи виден читателям
	bob.req("GET", p("/routes/%s", rid), nil, 200)
	if prev := bob.req("POST", p("/articles/preview"), map[string]any{"body": "## Заголовок\n**жирный**"}, 200); len(prev["blocks"].([]any)) != 2 {
		t.Fatalf("preview: %v", prev)
	}

	// блокировка и модерация
	anna.req("POST", p("/users/%d/block", eve.ID), nil, 204)
	eve.req("GET", p("/articles/%d", aid), nil, 404)
	bob.req("POST", p("/reports"), map[string]any{"target_type": "article", "target_id": aid, "reason": "спам"}, 201)
	mod.req("POST", p("/mod/articles/%d/hide", aid), nil, 204)
	bob.req("GET", p("/articles/%d", aid), nil, 404)

	anna.req("DELETE", p("/articles/%d", aid), nil, 204)
	if n := len(items(anna.req("GET", p("/articles/mine"), nil, 200))); n != 0 {
		t.Fatalf("mine after delete: %d", n)
	}
}
