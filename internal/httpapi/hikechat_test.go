package httpapi

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// Чат похода: создаётся сам, состав — организатор и все, кто идёт; очередь
// и заявки — не в чате; ушёл из похода — ушёл из чата.
func TestHikeChat(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	org, bob, carol, dan := register(t, ts, "org"), register(t, ts, "bob"), register(t, ts, "carol"), register(t, ts, "dan")
	start := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	h := org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай за два дня", "starts_at": start, "capacity": 2}, 201)
	hid := int64(h["id"].(float64))
	if h["chat_id"] == nil {
		t.Fatalf("no chat for the organizer: %v", h)
	}
	chat := int64(h["chat_id"].(float64))

	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)
	carol.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200) // мест нет — очередь
	if got := memberList(t, org, chat); got != "bob,org" {
		t.Fatalf("members: %s", got)
	}
	if got := bob.req("GET", p("/events/%d", hid), nil, 200); got["chat_id"] == nil || int64(got["chat_id"].(float64)) != chat {
		t.Fatalf("bob sees chat: %v", got["chat_id"])
	}
	if got := carol.req("GET", p("/events/%d", hid), nil, 200); got["chat_id"] != nil {
		t.Fatalf("waitlisted sees chat: %v", got["chat_id"])
	}
	carol.req("GET", p("/conversations/%d/messages", chat), nil, 404)

	// в списке диалогов — с привязкой к походу
	var found bool
	for _, c := range items(bob.req("GET", p("/conversations"), nil, 200)) {
		if cm := c.(map[string]any); int64(cm["id"].(float64)) == chat {
			found = cm["event_id"] != nil && int64(cm["event_id"].(float64)) == hid && cm["title"] == "Таганай за два дня"
		}
	}
	if !found {
		t.Fatal("hike chat missing in bob's conversations")
	}
	bob.req("POST", p("/conversations/%d/messages", chat), map[string]any{"body": "Беру горелку"}, 201)

	// Боб передумал — место Кэрол, и в чате тоже
	bob.req("DELETE", p("/events/%d/rsvp", hid), nil, 204)
	if got := memberList(t, org, chat); got != "carol,org" {
		t.Fatalf("after bob left: %s", got)
	}
	bob.req("GET", p("/conversations/%d/messages", chat), nil, 404)
	msgs := items(carol.req("GET", p("/conversations/%d/messages", chat), nil, 200))
	var texts []string
	for _, m := range msgs {
		texts = append(texts, m.(map[string]any)["body"].(string))
	}
	all := strings.Join(texts, " | ")
	for _, want := range []string{"чат похода «Таганай за два дня»", "bob в группе похода", "Беру горелку", "bob больше не в группе похода"} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q in %s", want, all)
		}
	}

	// состав руками не меняется: ни добавить, ни выйти, ни переименовать
	org.req("POST", p("/conversations/%d/members", chat), map[string]any{"user_id": dan.ID}, 409)
	carol.req("DELETE", p("/conversations/%d/members/%d", chat, carol.ID), nil, 409)
	org.req("PATCH", p("/conversations/%d", chat), map[string]any{"title": "другое"}, 409)

	// организатор убрал участника; переименовал поход — чат тоже
	org.req("DELETE", p("/events/%d/attendees/%d", hid, carol.ID), nil, 204)
	org.req("PATCH", p("/events/%d", hid), map[string]any{"title": "Таганай: Откликной гребень"}, 200)
	if got := memberList(t, org, chat); got != "org" {
		t.Fatalf("after removal: %s", got)
	}
	var title string
	_ = srv.store.Pool().QueryRow(t.Context(), `SELECT title FROM conversations WHERE id = $1`, chat).Scan(&title)
	if title != "Таганай: Откликной гребень" {
		t.Errorf("chat title: %q", title)
	}

	// у обычной встречи чата нет; у старого похода появляется при заходе
	ev := org.req("POST", p("/events"), map[string]any{"title": "Встреча клуба", "starts_at": start}, 201)
	if ev["chat_id"] != nil {
		t.Errorf("event got a chat: %v", ev)
	}
	old := org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Старый поход", "starts_at": start}, 201)
	if _, err := srv.store.Pool().Exec(t.Context(), `DELETE FROM conversations WHERE event_id = $1`, old["id"]); err != nil {
		t.Fatal(err)
	}
	if got := org.req("GET", p("/events/%v", old["id"]), nil, 200); got["chat_id"] == nil {
		t.Error("old hike did not get a chat")
	}

	// поход удалён — чат остаётся обычной беседой
	org.req("DELETE", p("/events/%d", hid), nil, 204)
	org.req("PATCH", p("/conversations/%d", chat), map[string]any{"title": "Архив Таганая"}, 204)
}

func memberList(t *testing.T, c *tuser, chat int64) string {
	t.Helper()
	var out []string
	for _, m := range items(c.req("GET", p("/conversations/%d/members", chat), nil, 200)) {
		out = append(out, m.(map[string]any)["username"].(string))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
