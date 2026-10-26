package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"socialnet/internal/store"
)

type ctxKey int

const viewerKey ctxKey = 1

// viewer — кто делает запрос.
type viewer struct {
	ID   int64
	Role string // user | moderator | admin
	SID  string // текущая сессия

	API    bool     // запрос с токеном приложения или бота
	Scopes []string // права токена API
	Bot    int64    // сообщество, если это бот
}

func viewerFrom(ctx context.Context) viewer {
	v, _ := ctx.Value(viewerKey).(viewer)
	return v
}

func userIDFrom(ctx context.Context) int64 { return viewerFrom(ctx).ID }

// requireAuth пускает дальше только запросы с валидным
// заголовком Authorization: Bearer <access token>.
// Бан, роль и активность сессии проверяются по базе на каждом запросе: забаненный теряет доступ
// сразу, а не когда истечёт его access-токен.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		if strings.HasPrefix(token, apiTokenPrefix) {
			s.apiTokenAuth(w, r, next, token)
			return
		}
		id, sid, err := s.tokens.ParseAccess(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}
		role, banned, sessionOK, err := s.store.AuthInfo(r.Context(), id, sid)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "user no longer exists")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if banned {
			writeError(w, http.StatusForbidden, "banned", "account is banned")
			return
		}
		// Сессию завершили (выход, «завершить все сеансы», смена пароля):
		// её access-токен перестаёт работать сразу.
		if !sessionOK {
			writeError(w, http.StatusUnauthorized, "session_ended", "session has ended, please log in again")
			return
		}
		if err := s.store.TouchLastSeen(r.Context(), id); err != nil {
			s.log.Warn("touch last seen", "err", err)
		}
		ctx := context.WithValue(r.Context(), viewerKey, viewer{ID: id, Role: role, SID: sid})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireRole пускает только модераторов/админов.
func requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := viewerFrom(r.Context())
			for _, role := range roles {
				if v.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
		})
	}
}

// securityHeaders: API отдаёт только JSON, поэтому политика максимально строгая.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// deadlines ставит дедлайны чтения и записи на конкретный запрос.
// Глобальные таймауты http.Server тут не годятся: они оборвали бы
// WebSocket и медленную загрузку фото. d == 0 снимает дедлайны.
func deadlines(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			var t time.Time
			if d > 0 {
				t = time.Now().Add(d)
			}
			_ = rc.SetReadDeadline(t)
			_ = rc.SetWriteDeadline(t)
			next.ServeHTTP(w, r)
		})
	}
}

func limitBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// ipRateLimiter: token bucket на каждый IP, хранится в памяти процесса.
// Для нескольких экземпляров API его стоит перенести в Redis.
type ipRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*client
	rps     rate.Limit
	burst   int
}

type client struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(rps float64, burst int) *ipRateLimiter {
	l := &ipRateLimiter{clients: map[string]*client{}, rps: rate.Limit(rps), burst: burst}
	go l.cleanup()
	return l
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.clients[ip]
	if !ok {
		c = &client{lim: rate.NewLimiter(l.rps, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = time.Now()
	return c.lim.Allow()
}

func (l *ipRateLimiter) cleanup() {
	for range time.Tick(time.Minute) {
		l.mu.Lock()
		for ip, c := range l.clients {
			if time.Since(c.lastSeen) > 10*time.Minute {
				delete(l.clients, ip)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP: адрес, определённый по X-Forwarded-For (при TRUST_PROXY), иначе
// адрес TCP-соединения. При TRUST_PROXY без годного XFF берётся адрес
// соединения — то есть самого прокси: лимит станет общим, но не обойдённым.
func clientIP(r *http.Request) string {
	if ip := middleware.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
