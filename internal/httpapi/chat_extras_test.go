package httpapi

import (
	"fmt"
	"socialnet/internal/store"
	"testing"
)

func TestChatPinsAndCustomStickers(t *testing.T) {
	s, other, a, b, _ := setupWallet(t)
	conv, e := s.store.Conversation(t.Context(), a.ID, b.ID)
	if e != nil {
		t.Fatal(e)
	}
	msg, e := s.store.SendMessage(t.Context(), store.NewMessage{ConversationID: conv, SenderID: a.ID, Body: "Pinned"})
	if e != nil {
		t.Fatal(e)
	}
	path := p(fmt.Sprintf("/conversations/%d/pin", conv))
	other.req("GET", path, nil, 404)
	a.req("PUT", path, map[string]any{"message_id": msg.ID}, 204)
	got := b.req("GET", path, nil, 200)
	if len(got["items"].([]any)) != 1 {
		t.Fatal(got)
	}
	otherConv, _ := s.store.Conversation(t.Context(), other.ID, b.ID)
	foreign, _ := s.store.SendMessage(t.Context(), store.NewMessage{ConversationID: otherConv, SenderID: b.ID, Body: "Private"})
	a.req("PUT", path, map[string]any{"message_id": foreign.ID}, 404)
	a.req("PUT", path, map[string]any{"message_id": 0}, 204)
	id, e := s.store.CreateMedia(t.Context(), a.ID, "image/png", 128, 128, 100)
	if e != nil {
		t.Fatal(e)
	}
	b.req("POST", p("/stickers"), map[string]any{"id": id, "title": "Stolen"}, 404)
	a.req("POST", p("/stickers"), map[string]any{"id": id, "title": "Hello"}, 204)
	got = a.req("GET", p("/stickers"), nil, 200)
	if len(got["items"].([]any)) != 1 {
		t.Fatal(got)
	}
	got = b.req("GET", p("/stickers"), nil, 200)
	if len(got["items"].([]any)) != 0 {
		t.Fatal(got)
	}
	a.req("POST", p("/stickers/remove"), map[string]any{"id": id}, 204)
}
