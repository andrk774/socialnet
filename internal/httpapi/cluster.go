package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"socialnet/internal/config"
	"socialnet/internal/realtime"
	"socialnet/internal/redisx"
	"socialnet/internal/s3"
)

// Несколько экземпляров API делят через Redis лимиты запросов, тикеты
// WebSocket и события (realtime.RedisBus). Без REDIS_URL всё живёт в памяти
// процесса — для одного экземпляра этого достаточно.

type limiter interface {
	allow(key string) bool
}

func limitMiddleware(l limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests, try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GCRA (generic cell rate algorithm) — тот же token bucket, но хранит одно
// число на ключ. Время берётся у Redis, так что часы экземпляров не важны.
const gcraScript = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local interval = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local tat = tonumber(redis.call('GET', KEYS[1])) or now
if tat < now then tat = now end
local new = tat + interval
if new - now > interval * burst then return 0 end
redis.call('SET', KEYS[1], new, 'PX', math.ceil(new - now) + 1)
return 1`

type redisLimiter struct {
	r        *redisx.Client
	name     string
	interval float64 // мс на один запрос
	burst    int
	local    *ipRateLimiter // если Redis недоступен — лимит хотя бы на этом экземпляре
	log      *slog.Logger
}

func newRedisLimiter(r *redisx.Client, log *slog.Logger, name string, rps float64, burst int) *redisLimiter {
	return &redisLimiter{r: r, name: name, interval: math.Ceil(1000 / rps), burst: burst,
		local: newIPRateLimiter(rps, burst), log: log}
}

func (l *redisLimiter) allow(key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	n, err := l.r.Int(ctx, "EVAL", gcraScript, 1, "sn:rl:"+l.name+":"+key, l.interval, l.burst)
	if err != nil {
		l.log.Warn("redis rate limit unavailable, using local", "limiter", l.name, "err", err)
		return l.local.allow(key)
	}
	return n == 1
}

// ---------- тикеты WebSocket ----------

type ticketStore interface {
	issue(userID int64) string
	consume(id string) (int64, bool)
}

type redisTickets struct {
	r     *redisx.Client
	local *wsTickets
	log   *slog.Logger
}

// В Redis лежит только sha256 тикета: дамп Redis не даёт рабочих пропусков.
func ticketKey(id string) string {
	h := sha256.Sum256([]byte(id))
	return "sn:wst:" + hex.EncodeToString(h[:])
}

func (t *redisTickets) issue(userID int64) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	id := base64.RawURLEncoding.EncodeToString(b)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := t.r.Do(ctx, "SET", ticketKey(id), strconv.FormatInt(userID, 10), "EX", 30, "NX"); err != nil {
		t.log.Warn("redis ws ticket, using local", "err", err)
		return t.local.issue(userID)
	}
	return id
}

func (t *redisTickets) consume(id string) (int64, bool) {
	if id == "" || len(id) > 100 {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := t.r.String(ctx, "GETDEL", ticketKey(id)) // одноразовость атомарно
	if errors.Is(err, redisx.ErrNil) {
		return t.local.consume(id) // мог быть выдан локально, пока Redis лежал
	}
	if err != nil {
		t.log.Warn("redis ws ticket", "err", err)
		return t.local.consume(id)
	}
	uid, err := strconv.ParseInt(v, 10, 64)
	return uid, err == nil
}

func (s *Server) limiter(name string, rps float64, burst int) limiter {
	if s.redis != nil {
		return newRedisLimiter(s.redis, s.log, name, rps, burst)
	}
	return newIPRateLimiter(rps, burst)
}

func newInstanceID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	host, _ := os.Hostname()
	if len(host) > 32 {
		host = host[:32]
	}
	return host + "-" + hex.EncodeToString(b)
}

// Start запускает фоновые части сервера: шину событий между экземплярами
// и доставку Callback API. Работает до отмены ctx.
func (s *Server) Start(ctx context.Context) {
	if s.redis != nil {
		s.hub.AttachBus(ctx, &realtime.RedisBus{R: s.redis, Instance: s.instance}, s.instance, s.log)
	}
	go s.RunCallbacks(ctx)
	go s.runDetector(ctx)
	go s.runScheduler(ctx)
}

// NewS3 — клиент объектного хранилища из настроек; nil, если S3 не настроен.
func NewS3(cfg config.Config) *s3.Client {
	if cfg.S3Endpoint == "" {
		return nil
	}
	return &s3.Client{Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle,
		HTTP: &http.Client{Timeout: 10 * time.Minute}}
}
