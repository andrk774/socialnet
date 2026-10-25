// Package realtime: доставка событий пользователям, подключённым по WebSocket.
//
// Хаб живёт в памяти процесса. Пока API работает в одном экземпляре, этого
// достаточно. Для нескольких экземпляров Publish нужно пустить через
// Redis pub/sub: интерфейс для обработчиков при этом не изменится.
package realtime

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Client — одно соединение. Пользователь может быть подключён
// с нескольких устройств сразу.
type Client struct {
	UserID int64
	Send   chan []byte
	once   sync.Once
}

func (c *Client) close() { c.once.Do(func() { close(c.Send) }) }

type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: map[int64]map[*Client]struct{}{}}
}

func (h *Hub) Subscribe(userID int64) *Client {
	c := &Client{UserID: userID, Send: make(chan []byte, 64)}
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = map[*Client]struct{}{}
	}
	h.clients[userID][c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	if set := h.clients[c.UserID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.UserID)
		}
	}
	h.mu.Unlock()
	c.close()
}

func (h *Hub) Online(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// Publish отправляет событие всем соединениям пользователя.
// Медленный клиент с переполненным буфером отключается, а не тормозит остальных.
// Возвращает true, если событие ушло хотя бы в одно соединение.
func (h *Hub) Publish(userID int64, typ string, data any) bool {
	if h == nil {
		return false
	}
	msg, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return false
	}
	h.mu.RLock()
	var slow []*Client
	delivered := false
	for c := range h.clients[userID] {
		select {
		case c.Send <- msg:
			delivered = true
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		h.Unsubscribe(c)
	}
	return delivered
}
