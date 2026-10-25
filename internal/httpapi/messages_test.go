package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func wsConnect(t *testing.T, ts *httptest.Server, u *tuser) *websocket.Conn {
	t.Helper()
	ticket := u.req("POST", p("/ws/ticket"), nil, 200)["ticket"].(string)
	url := strings.Replace(ts.URL, "http", "ws", 1) + "/api/v1/ws?ticket=" + ticket
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

// waitEvent читает события, пока не придёт нужный тип.
func waitEvent(t *testing.T, conn *websocket.Conn, typ string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v", typ, err)
		}
		var ev struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(data, &ev)
		if ev.Type == typ {
			return ev.Data
		}
	}
}

func wsSend(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := conn.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func TestMessagingRealtime(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	annaWS, bobWS := wsConnect(t, ts, anna), wsConnect(t, ts, bob)

	msg := anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": "Привет, Боб!"}, 201)
	convID := int64(msg["conversation_id"].(float64))
	msgID := msg["id"].(float64)

	got := waitEvent(t, bobWS, "message")
	if got["body"] != "Привет, Боб!" || got["sender_id"].(float64) != float64(anna.ID) {
		t.Fatalf("bob got: %v", got)
	}
	if d := waitEvent(t, annaWS, "delivered"); d["up_to"].(float64) != msgID {
		t.Fatalf("delivered: %v", d)
	}

	// у Боба 1 непрочитанное
	convs := items(bob.req("GET", p("/conversations"), nil, 200))
	if len(convs) != 1 || convs[0].(map[string]any)["unread"].(float64) != 1 {
		t.Fatalf("conversations: %v", convs)
	}

	// «печатает…» по WebSocket
	wsSend(t, bobWS, map[string]any{"type": "typing", "conversation_id": convID})
	if ty := waitEvent(t, annaWS, "typing"); ty["user_id"].(float64) != float64(bob.ID) {
		t.Fatalf("typing: %v", ty)
	}

	// прочитано
	bob.req("POST", p("/conversations/%d/read", convID), map[string]any{"up_to": msgID}, 200)
	if rd := waitEvent(t, annaWS, "read"); rd["up_to"].(float64) != msgID {
		t.Fatalf("read: %v", rd)
	}
	convs = items(anna.req("GET", p("/conversations"), nil, 200))
	if convs[0].(map[string]any)["peer_read_id"].(float64) != msgID {
		t.Fatalf("peer_read_id: %v", convs[0])
	}
	// отметку нельзя увести дальше последнего сообщения
	if out := bob.req("POST", p("/conversations/%d/read", convID), map[string]any{"up_to": 1 << 40}, 200); out["last_read_id"].(float64) != msgID {
		t.Fatalf("read beyond last: %v", out)
	}

	// чужой диалог недоступен (IDOR)
	eve.req("GET", p("/conversations/%d/messages", convID), nil, 404)
	eve.req("POST", p("/conversations/%d/read", convID), map[string]any{"up_to": msgID}, 404)
	eve.req("POST", p("/conversations/%d/typing", convID), nil, 404)
	if n := len(items(bob.req("GET", p("/conversations/%d/messages", convID), nil, 200))); n != 1 {
		t.Fatalf("messages: %d", n)
	}

	// уведомление тоже приходит по WebSocket
	eve.req("POST", p("/users/%d/friend", bob.ID), nil, 200)
	if n := waitEvent(t, bobWS, "notification"); n["type"] != "friend_request" {
		t.Fatalf("notification: %v", n)
	}

	anna.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "себе"}, 400)
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": ""}, 422)
}

func TestMessagePrivacy(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)
	anna.req("PATCH", p("/users/me/privacy"), map[string]any{"message_privacy": "friends"}, 200)

	bob.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "привет"}, 201)
	eve.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "привет"}, 403)

	anna.req("POST", p("/users/%d/block", bob.ID), nil, 204)
	bob.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "ну привет"}, 403)
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": "и я тебе не пишу"}, 403)

	anna.req("PATCH", p("/users/me/privacy"), map[string]any{"message_privacy": "nobody"}, 200)
	anna.req("DELETE", p("/users/%d/block", bob.ID), nil, 204)
	bob.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "?"}, 403)
}

func TestWSTicket(t *testing.T) {
	ts := setup(t, 1000)
	anna := register(t, ts, "anna")
	ticket := anna.req("POST", p("/ws/ticket"), nil, 200)["ticket"].(string)
	url := strings.Replace(ts.URL, "http", "ws", 1) + "/api/v1/ws?ticket=" + ticket

	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.CloseNow()
	// повторное использование тикета
	if _, resp, err := websocket.Dial(context.Background(), url, nil); err == nil || resp.StatusCode != 401 {
		t.Fatal("ticket reused")
	}
	if _, resp, err := websocket.Dial(context.Background(), strings.Replace(ts.URL, "http", "ws", 1)+"/api/v1/ws", nil); err == nil || resp.StatusCode != 401 {
		t.Fatal("no ticket accepted")
	}
	// чужой сайт (Cross-Site WebSocket Hijacking)
	ticket = anna.req("POST", p("/ws/ticket"), nil, 200)["ticket"].(string)
	h := http.Header{"Origin": {"https://evil.example"}}
	if _, _, err := websocket.Dial(context.Background(), strings.Replace(ts.URL, "http", "ws", 1)+"/api/v1/ws?ticket="+ticket,
		&websocket.DialOptions{HTTPHeader: h}); err == nil {
		t.Fatal("cross-origin websocket accepted")
	}
}

func TestEncryptedMessages(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")

	key := bytes.Repeat([]byte{7}, 32)
	bob.req("PUT", p("/users/me/keys"), map[string]any{"identity_key": key}, 204)
	bob.req("PUT", p("/users/me/keys"), map[string]any{"identity_key": []byte("short")}, 422)
	got := anna.req("GET", p("/users/%d/keys", bob.ID), nil, 200)
	if got["identity_key"] == nil {
		t.Fatalf("key: %v", got)
	}
	anna.req("GET", p("/users/%d/keys", anna.ID), nil, 404)

	ct := bytes.Repeat([]byte{1, 2, 3}, 20)
	msg := anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"ciphertext": ct, "nonce": []byte("123456789012")}, 201)
	if msg["encrypted"] != true || msg["body"] != nil {
		t.Fatalf("encrypted message: %v", msg)
	}
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"body": "x", "ciphertext": ct, "nonce": []byte("n")}, 422)
	anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"ciphertext": ct}, 422)
}
