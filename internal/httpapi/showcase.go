package httpapi

import (
	"net/http"
	"sync"
	"time"
)

// Витрина для гостей на главной: ближайшие походы и свежие маршруты
// каталога. Без входа, поэтому — только то, что не раскрывает людей:
// без мест сбора, описаний и списков участников. Ответ кешируется на
// минуту: главную открывают чаще всего.

type showcaseHike struct {
	ID         int64     `json:"id"`
	Title      string    `json:"title"`
	Place      string    `json:"place"`
	StartsAt   time.Time `json:"starts_at"`
	Difficulty *int      `json:"difficulty"`
	Capacity   *int      `json:"capacity"`
	Going      int       `json:"going_count"`
}

type showcase struct {
	Hikes  []showcaseHike `json:"hikes"`
	Routes []searchRoute  `json:"routes"`
}

type showcaseCache struct {
	mu   sync.Mutex
	at   time.Time
	data *showcase
	ttl  time.Duration // 0 — минута; в тестах — без кеша
}

func (s *Server) handleShowcase(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
		return
	}
	c := &s.showcase
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := c.ttl
	if ttl == 0 {
		ttl = time.Minute
	}
	if c.data == nil || time.Since(c.at) > ttl {
		hikes, err := s.store.Hikes(r.Context(), 0, 0, 6)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		routes, err := s.store.PublicRoutes(r.Context(), 6, 0)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		d := &showcase{Hikes: []showcaseHike{}, Routes: []searchRoute{}}
		for _, e := range hikes {
			d.Hikes = append(d.Hikes, showcaseHike{ID: e.ID, Title: e.Title, Place: e.Place, StartsAt: e.StartsAt,
				Difficulty: e.Difficulty, Capacity: e.Capacity, Going: e.Going})
		}
		for _, pr := range routes {
			d.Routes = append(d.Routes, searchRoute{ID: pr.ID, Title: pr.Title, DistanceM: pr.DistanceM, GainM: pr.GainM, Author: pr.Author, URL: "/trails/" + pr.ID})
		}
		c.data, c.at = d, time.Now()
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, c.data)
}
