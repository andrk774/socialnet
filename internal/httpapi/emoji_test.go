package httpapi

import (
	"fmt"
	"testing"
)

func TestCommunityEmoji(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	owner, mem, bob := register(t, ts, "owner"), register(t, ts, "member"), register(t, ts, "bob")
	c := owner.req("POST", p("/communities"), map[string]any{"slug": "taganay_club", "name": "Клуб Таганая"}, 201)
	cid := int64(c["id"].(float64))
	other := owner.req("POST", p("/communities"), map[string]any{"slug": "iremel_club", "name": "Иремель"}, 201)
	mem.req("POST", p("/communities/%d/join", cid), nil, 204)
	_, img := upload(t, ts, owner, pngBytes(64, 64))
	_, memImg := upload(t, ts, mem, pngBytes(64, 64))

	mem.req("POST", p("/communities/%d/emoji", cid), map[string]any{"shortcode": "gora", "media_id": memImg["id"]}, 403)
	owner.req("POST", p("/communities/%d/emoji", cid), map[string]any{"shortcode": "Гора!", "media_id": img["id"]}, 422)
	owner.req("POST", p("/communities/%d/emoji", cid), map[string]any{"shortcode": "gora", "media_id": memImg["id"]}, 422) // чужое фото
	e := owner.req("POST", p("/communities/%d/emoji", cid), map[string]any{"shortcode": ":Gora:", "media_id": img["id"]}, 201)
	owner.req("POST", p("/communities/%d/emoji", cid), map[string]any{"shortcode": "gora", "media_id": img["id"]}, 409)
	if e["shortcode"] != "gora" || e["reaction"] != fmt.Sprintf("e:%d", int64(e["id"].(float64))) {
		t.Fatalf("emoji: %v", e)
	}
	reaction := e["reaction"].(string)
	if n := len(items(bob.req("GET", p("/communities/%d/emoji", cid), nil, 200))); n != 1 {
		t.Fatalf("list: %d", n)
	}

	cpost := owner.req("POST", p("/posts"), map[string]any{"text": "Сбор в субботу", "community_id": cid}, 201)
	opost := owner.req("POST", p("/posts"), map[string]any{"text": "Чужое сообщество", "community_id": int64(other["id"].(float64))}, 201)
	upost := bob.req("POST", p("/posts"), map[string]any{"text": "Мой пост"}, 201)
	// эмодзи сообщества — только на его записях
	mem.req("POST", p("/posts/%d/reactions", int64(opost["id"].(float64))), map[string]any{"reaction": reaction}, 422)
	mem.req("POST", p("/posts/%d/reactions", int64(upost["id"].(float64))), map[string]any{"reaction": reaction}, 422)
	mem.req("POST", p("/posts/%d/reactions", int64(cpost["id"].(float64))), map[string]any{"reaction": "e:999999"}, 422)
	mem.req("POST", p("/posts/%d/reactions", int64(cpost["id"].(float64))), map[string]any{"reaction": reaction}, 200)
	got := bob.req("GET", p("/posts/%d", int64(cpost["id"].(float64))), nil, 200)
	if got["reactions"].(map[string]any)[reaction].(float64) != 1 {
		t.Fatalf("reactions: %v", got["reactions"])
	}
	// удалили эмодзи — реакция стала «нравится», счётчик тот же
	mem.req("DELETE", p("/communities/%d/emoji/%d", cid, int64(e["id"].(float64))), nil, 403)
	owner.req("DELETE", p("/communities/%d/emoji/%d", cid, int64(e["id"].(float64))), nil, 204)
	got = bob.req("GET", p("/posts/%d", int64(cpost["id"].(float64))), nil, 200)
	if got["likes_count"].(float64) != 1 || got["reactions"].(map[string]any)["like"].(float64) != 1 {
		t.Fatalf("after delete: %v %v", got["likes_count"], got["reactions"])
	}
}
