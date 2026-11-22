package httpapi

import "testing"

func TestEditMessage(t *testing.T) {
	ts := setup(t, 1000)
	a, b, c := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	msg := a.req("POST", p("/users/%d/messages", b.ID), map[string]any{"body": "original"}, 201)
	cid, mid := int64(msg["conversation_id"].(float64)), int64(msg["id"].(float64))
	endpoint := p("/conversations/%d/messages/%d", cid, mid)
	b.req("PUT", endpoint, map[string]any{"body": "stolen"}, 404)
	c.req("PUT", endpoint, map[string]any{"body": "stolen"}, 404)
	a.req("PUT", endpoint, map[string]any{"body": " "}, 422)
	ws := wsConnect(t, ts, b)
	got := a.req("PUT", endpoint, map[string]any{"body": "updated"}, 200)
	if got["body"] != "updated" || got["edited_at"] == nil {
		t.Fatal(got)
	}
	if event := waitEvent(t, ws, "message_updated"); event["body"] != "updated" {
		t.Fatal(event)
	}
	b.req("DELETE", endpoint, nil, 404)
	got = a.req("DELETE", endpoint, nil, 200)
	if got["deleted_at"] == nil || len(got["attachments"].([]any)) != 0 {
		t.Fatal(got)
	}
	a.req("PUT", endpoint, map[string]any{"body": "revive"}, 404)
}
