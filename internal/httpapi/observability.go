package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"socialnet/internal/metrics"
	"socialnet/internal/store"
)

// ---------- метрики ----------

type serverMetrics struct {
	reg      *metrics.Registry
	requests *metrics.CounterVec
	latency  *metrics.HistogramVec
	events   *metrics.CounterVec // события безопасности: login, login_failed, ...
	business *metrics.CounterVec // сообщения, посты, покупки, доставки callback
	alerts   *metrics.CounterVec
}

func (s *Server) initMetrics() {
	reg := metrics.NewRegistry()
	m := &serverMetrics{
		reg:      reg,
		requests: reg.Counter("socialnet_http_requests_total", "HTTP-запросы по маршруту и коду ответа.", "method", "route", "status"),
		latency: reg.Histogram("socialnet_http_request_duration_seconds", "Время обработки запроса.",
			metrics.DefaultBuckets, "method", "route"),
		events:   reg.Counter("socialnet_security_events_total", "События журнала безопасности.", "type"),
		business: reg.Counter("socialnet_events_total", "Бизнес-события.", "event"),
		alerts:   reg.Counter("socialnet_security_alerts_total", "Срабатывания детектора аномалий.", "rule", "severity"),
	}
	reg.Gauge("socialnet_ws_connections", "Открытые WebSocket-соединения на этом экземпляре.",
		func() float64 { return float64(s.hub.Connections()) })
	reg.GaugeVec("socialnet_db_pool_connections", "Соединения пула PostgreSQL.", "state", s.store.PoolStats)
	reg.GaugeVec("socialnet_queue_depth", "Ожидающие задачи и открытые тревоги.", "queue", func() map[string]float64 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		v, err := s.store.QueueDepth(ctx)
		if err != nil {
			return map[string]float64{}
		}
		return v
	})
	reg.GaugeVec("socialnet_backup", "Резервные копии: время последней удачной (unix), последнего запуска, итог проверки восстановлением.",
		"metric", func() map[string]float64 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return s.store.BackupStatus(ctx)
		})
	s.m = m
}

// instrument считает запросы. Метка route — шаблон маршрута, а не путь:
// иначе каждый /users/123 дал бы свою серию (взрыв кардинальности).
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		s.m.requests.Inc(r.Method, route, strconv.Itoa(status))
		s.m.latency.Observe(time.Since(start).Seconds(), r.Method, route)
	})
}

// MetricsHandler — /metrics для Prometheus. Слушает отдельный внутренний
// адрес (METRICS_ADDR); METRICS_TOKEN — дополнительно Bearer-токен.
func (s *Server) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := s.cfg.MetricsToken; tok != "" {
			got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(tok)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		s.m.reg.WriteText(w)
	})
}

// ---------- журнал действий администраторов ----------

// audit записывает каждый запрос к /mod и /admin: кто, что, с каким
// телом, откуда и с каким результатом. Пишется и при ошибке — попытки тоже важны.
func (s *Server) audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil && r.Method != http.MethodGet {
			body, _ = io.ReadAll(io.LimitReader(r.Body, 4096))
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
		}
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		v := viewerFrom(r.Context())
		route := r.URL.Path
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		var details json.RawMessage
		if json.Valid(body) {
			details = body
		}
		// Запись идёт после ответа, но в том же запросе: не теряется, если процесс жив.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
		if err := s.store.AppendAudit(ctx, store.AuditEntry{ActorID: v.ID, ActorRole: v.Role,
			Action: r.Method + " " + route, Target: r.URL.Path, Details: details, Status: status,
			IP: clientIP(r), UserAgent: r.UserAgent(), RequestID: middleware.GetReqID(r.Context())}); err != nil {
			s.log.Error("audit log write failed", "err", err, "action", r.Method+" "+route)
		}
	})
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	pg := pageParams(r)
	actor, _ := strconv.ParseInt(r.URL.Query().Get("actor_id"), 10, 64)
	entries, err := s.store.AuditLog(r.Context(), actor, pg.Before, pg.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(entries, pg.Limit, func(e store.AuditEntry) int64 { return e.ID }))
}

func (s *Server) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	checked, broken, err := s.store.VerifyAudit(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if broken != 0 {
		s.log.Error("audit chain broken", "at", broken)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": broken == 0, "checked": checked, "broken_at": broken})
}

// ---------- детектор аномалий ----------

const alertCols = `RETURNING id, rule, subject, severity, details, created_at`

// detectorRules — простые и объяснимые правила поверх журналов. Каждое
// срабатывает один раз на субъект за окно (UNIQUE rule+subject+window).
var detectorRules = []store.Rule{
	{Name: "login_bruteforce", Window: 15 * time.Minute, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT 'login_bruteforce', 'user:' || user_id, 'medium',
		       jsonb_build_object('failures', count(*), 'distinct_ips', count(DISTINCT ip)), $1::timestamptz
		FROM security_events
		WHERE type IN ('login_failed', '2fa_failed') AND created_at > now() - interval '15 minutes'
		GROUP BY user_id HAVING count(*) >= 10
		ON CONFLICT DO NOTHING ` + alertCols},
	{Name: "password_spray", Window: 15 * time.Minute, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT 'password_spray', 'ip:' || host(ip), 'high',
		       jsonb_build_object('accounts', count(DISTINCT user_id), 'failures', count(*)), $1::timestamptz
		FROM security_events
		WHERE type = 'login_failed' AND ip IS NOT NULL AND created_at > now() - interval '15 minutes'
		GROUP BY ip HAVING count(DISTINCT user_id) >= 5
		ON CONFLICT DO NOTHING ` + alertCols},
	{Name: "token_reuse", Window: 15 * time.Minute, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT 'token_reuse', 'user:' || user_id, 'high', jsonb_build_object('events', count(*)), $1::timestamptz
		FROM security_events
		WHERE type = 'refresh_token_reuse' AND created_at > now() - interval '15 minutes'
		GROUP BY user_id
		ON CONFLICT DO NOTHING ` + alertCols},
	{Name: "admin_burst", Window: 10 * time.Minute, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT 'admin_burst', 'actor:' || actor_id, 'high',
		       jsonb_build_object('actions', count(*), 'kinds', count(DISTINCT action)), $1::timestamptz
		FROM audit_log
		WHERE action NOT LIKE 'GET %' AND created_at > now() - interval '10 minutes'
		GROUP BY actor_id HAVING count(*) >= 30
		ON CONFLICT DO NOTHING ` + alertCols},
	{Name: "admin_new_ip", Window: 24 * time.Hour, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT DISTINCT 'admin_new_ip', 'actor:' || a.actor_id || ':' || host(a.ip), 'medium',
		       jsonb_build_object('action', a.action), $1::timestamptz
		FROM audit_log a
		WHERE a.created_at > now() - interval '15 minutes' AND a.ip IS NOT NULL
		  AND EXISTS (SELECT 1 FROM audit_log o WHERE o.actor_id = a.actor_id AND o.id < a.id)
		  AND NOT EXISTS (SELECT 1 FROM audit_log o WHERE o.actor_id = a.actor_id AND o.id < a.id
		                  AND o.ip = a.ip AND o.created_at > now() - interval '90 days')
		ON CONFLICT DO NOTHING ` + alertCols},
	{Name: "mint_large", Window: time.Hour, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT 'mint_large', 'mint', 'high', jsonb_build_object('coins', sum(amount), 'operations', count(*)), $1::timestamptz
		FROM wallet_transactions
		WHERE kind = 'mint' AND created_at > now() - interval '1 hour'
		HAVING sum(amount) >= 100000
		ON CONFLICT DO NOTHING ` + alertCols},
	{Name: "transfer_burst", Window: time.Hour, SQL: `
		INSERT INTO security_alerts (rule, subject, severity, details, window_start)
		SELECT 'transfer_burst', 'user:' || a.owner_id, 'medium',
		       jsonb_build_object('transfers', count(*), 'coins', sum(t.amount)), $1::timestamptz
		FROM wallet_transactions t JOIN accounts a ON a.id = t.from_account AND a.kind = 'user'
		WHERE t.kind IN ('transfer', 'gift', 'donation') AND t.created_at > now() - interval '1 hour'
		GROUP BY a.owner_id HAVING count(*) >= 30 OR sum(t.amount) >= 50000
		ON CONFLICT DO NOTHING ` + alertCols},
}

// RunDetectorOnce проверяет правила; новые тревоги — в лог, метрики и на
// почту ответственного за безопасность.
func (s *Server) RunDetectorOnce(ctx context.Context) ([]store.Alert, error) {
	alerts, err := s.store.RunRules(ctx, detectorRules)
	for _, a := range alerts {
		s.m.alerts.Inc(a.Rule, a.Severity)
		s.log.Warn("security alert", "rule", a.Rule, "subject", a.Subject, "severity", a.Severity, "details", string(a.Details))
	}
	if len(alerts) > 0 && s.cfg.SecurityContact != "" {
		var b strings.Builder
		for _, a := range alerts {
			fmt.Fprintf(&b, "[%s] %s — %s %s\n", a.Severity, a.Rule, a.Subject, a.Details)
		}
		b.WriteString("\nПодробности: " + s.cfg.AppURL + "/admin/alerts\n")
		s.sendMail(s.cfg.SecurityContact, fmt.Sprintf("Socialnet: %d новых тревог безопасности", len(alerts)), b.String())
	}
	return alerts, err
}

func (s *Server) runDetector(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if _, err := s.RunDetectorOnce(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("anomaly detector", "err", err)
		}
	}
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	pg := pageParams(r)
	alerts, err := s.store.Alerts(r.Context(), r.URL.Query().Get("status") != "all", pg.Before, pg.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(alerts, pg.Limit, func(a store.Alert) int64 { return a.ID }))
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := s.store.AckAlert(r.Context(), id, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "alert not found or already acknowledged")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
