// Package realtime: доставка событий пользователям, подключённым по WebSocket.
//
// Соединения живут в памяти процесса. Если API запущено в нескольких
// экземплярах, хаб подключается к шине (Redis pub/sub): событие уходит
// своим соединениям и в шину, а другие экземпляры доставляют его своим.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
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

// Bus — общая шина между экземплярами.
type Bus interface {
	Publish(ctx context.Context, payload []byte) error
	Subscribe(ctx context.Context) <-chan []byte
	// SetOnline отмечает, что у пользователя есть (или больше нет) соединения на этом экземпляре.
	SetOnline(ctx context.Context, userID int64, online bool) error
	// RemoteOnline — есть ли соединения пользователя на других живых экземплярах.
	RemoteOnline(ctx context.Context, userID int64) bool
	Heartbeat(ctx context.Context) error
}

// envelope — событие в шине. I — экземпляр-отправитель (свои не доставляем дважды).
type envelope struct {
	I string          `json:"i"`
	U int64           `json:"u"`
	M json.RawMessage `json:"m"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*Client]struct{}

	bus      Bus
	instance string
	log      *slog.Logger
}

func NewHub() *Hub {
	return &Hub{clients: map[int64]map[*Client]struct{}{}}
}

// AttachBus подключает шину: слушает события других экземпляров и
// поддерживает отметку «экземпляр жив». Работает до отмены ctx.
func (h *Hub) AttachBus(ctx context.Context, bus Bus, instance string, log *slog.Logger) {
	h.mu.Lock()
	h.bus, h.instance, h.log = bus, instance, log
	h.mu.Unlock()
	_ = bus.Heartbeat(ctx)
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := bus.Heartbeat(ctx); err != nil && ctx.Err() == nil {
					log.Warn("realtime heartbeat", "err", err)
				}
			}
		}
	}()
	msgs := bus.Subscribe(ctx)
	go func() {
		for payload := range msgs {
			var env envelope
			if json.Unmarshal(payload, &env) != nil || env.I == instance {
				continue
			}
			h.deliverLocal(env.U, env.M)
		}
	}()
}

func (h *Hub) Subscribe(userID int64) *Client {
	c := &Client{UserID: userID, Send: make(chan []byte, 64)}
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = map[*Client]struct{}{}
	}
	h.clients[userID][c] = struct{}{}
	first := len(h.clients[userID]) == 1
	bus := h.bus
	h.mu.Unlock()
	if first && bus != nil {
		h.setOnline(bus, userID, true)
	}
	return c
}

func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	last := false
	if set := h.clients[c.UserID]; set != nil {
		if _, ok := set[c]; ok {
			delete(set, c)
			if len(set) == 0 {
				delete(h.clients, c.UserID)
				last = true
			}
		}
	}
	bus := h.bus
	h.mu.Unlock()
	c.close()
	if last && bus != nil {
		h.setOnline(bus, c.UserID, false)
	}
}

func (h *Hub) setOnline(bus Bus, userID int64, on bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bus.SetOnline(ctx, userID, on); err != nil && h.log != nil {
		h.log.Warn("realtime presence", "err", err)
	}
}

// Connections — число открытых соединений на этом экземпляре (для метрик).
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, set := range h.clients {
		n += len(set)
	}
	return n
}

func (h *Hub) localOnline(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

func (h *Hub) remoteOnline(userID int64) bool {
	h.mu.RLock()
	bus := h.bus
	h.mu.RUnlock()
	if bus == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return bus.RemoteOnline(ctx, userID)
}

// Online — подключён ли пользователь к любому экземпляру.
func (h *Hub) Online(userID int64) bool {
	return h.localOnline(userID) || h.remoteOnline(userID)
}

// deliverLocal: медленный клиент с переполненным буфером отключается,
// а не тормозит остальных.
func (h *Hub) deliverLocal(userID int64, msg []byte) bool {
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

// Encode — событие в формате шины; им пользуется воркер, у которого нет хаба.
func Encode(instance string, userID int64, typ string, data any) ([]byte, error) {
	msg, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{I: instance, U: userID, M: msg})
}

// Publish отправляет событие всем соединениям пользователя на всех экземплярах.
// Возвращает true, если пользователь сейчас подключён (значит, push не нужен).
func (h *Hub) Publish(userID int64, typ string, data any) bool {
	if h == nil {
		return false
	}
	msg, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return false
	}
	delivered := h.deliverLocal(userID, msg)
	h.mu.RLock()
	bus, inst := h.bus, h.instance
	h.mu.RUnlock()
	if bus == nil {
		return delivered
	}
	env, _ := json.Marshal(envelope{I: inst, U: userID, M: msg})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Publish(ctx, env); err != nil && h.log != nil {
		h.log.Warn("realtime publish", "err", err)
	}
	return delivered || bus.RemoteOnline(ctx, userID)
}
