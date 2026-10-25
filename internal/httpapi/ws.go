package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// wsTickets — одноразовые пропуска для WebSocket.
// Браузер не умеет ставить заголовок Authorization при открытии WebSocket,
// а класть JWT в URL опасно (URL попадает в логи). Поэтому клиент сначала
// получает тикет обычным запросом с токеном, а затем подключается с ним.
// Тикет живёт 30 секунд и сгорает при первом использовании.
type wsTickets struct {
	mu sync.Mutex
	m  map[string]wsTicket
}

type wsTicket struct {
	userID int64
	exp    time.Time
}

func newWSTickets() *wsTickets { return &wsTickets{m: map[string]wsTicket{}} }

func (t *wsTickets) issue(userID int64) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	id := base64.RawURLEncoding.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for k, v := range t.m { // заодно чистим просроченные
		if now.After(v.exp) {
			delete(t.m, k)
		}
	}
	t.m[id] = wsTicket{userID: userID, exp: now.Add(30 * time.Second)}
	return id
}

func (t *wsTickets) consume(id string) (int64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.m[id]
	delete(t.m, id)
	if !ok || time.Now().After(v.exp) {
		return 0, false
	}
	return v.userID, true
}

func (s *Server) handleWSTicket(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"ticket": s.wsTickets.issue(userIDFrom(r.Context()))})
}

// wsInbound — что клиент может прислать по WebSocket.
type wsInbound struct {
	Type           string `json:"type"` // typing | delivered
	ConversationID int64  `json:"conversation_id"`
	UpTo           int64  `json:"up_to"`
}

// handleWS: GET /api/v1/ws?ticket=...
// Сервер шлёт события: message, delivered, read, typing, notification.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.wsTickets.consume(r.URL.Query().Get("ticket"))
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired ticket")
		return
	}
	// Проверка Origin защищает от Cross-Site WebSocket Hijacking:
	// чужой сайт не сможет открыть соединение из браузера жертвы.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.AllowedOrigins})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(4096)

	client := s.hub.Subscribe(userID)
	defer s.hub.Unsubscribe(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// писатель: события из хаба + ping каждые 30 секунд
	go func() {
		defer cancel()
		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()
		for {
			select {
			case msg, ok := <-client.Send:
				if !ok {
					_ = conn.Close(websocket.StatusPolicyViolation, "too slow")
					return
				}
				wctx, c := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				c()
				if err != nil {
					return
				}
			case <-ping.C:
				pctx, c := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Ping(pctx)
				c()
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	var lastTyping time.Time
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var in wsInbound
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		conv, err := s.store.ConvAccess(ctx, in.ConversationID, userID)
		if err != nil {
			continue // чужой или несуществующий диалог — молча игнорируем
		}
		others := func(typ string, data any) {
			for _, uid := range conv.Members {
				if uid != userID {
					s.hub.Publish(uid, typ, data)
				}
			}
		}
		switch in.Type {
		case "typing":
			if time.Since(lastTyping) < 2*time.Second {
				continue // не чаще раза в 2 секунды
			}
			lastTyping = time.Now()
			others("typing", map[string]int64{"conversation_id": in.ConversationID, "user_id": userID})
		case "delivered":
			if up, err := s.store.MarkDelivered(ctx, in.ConversationID, userID, in.UpTo); err == nil {
				others("delivered", map[string]int64{"conversation_id": in.ConversationID, "user_id": userID, "up_to": up})
			}
		}
	}
}
