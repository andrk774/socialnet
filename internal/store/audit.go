package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type AuditEntry struct {
	ID        int64           `json:"id"`
	ActorID   int64           `json:"actor_id"`
	ActorRole string          `json:"actor_role"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	Details   json.RawMessage `json:"details,omitempty"`
	Status    int             `json:"status"`
	IP        string          `json:"ip"`
	UserAgent string          `json:"user_agent"`
	RequestID string          `json:"request_id"`
	CreatedAt time.Time       `json:"created_at"`
	PrevHash  []byte          `json:"-"`
	Hash      []byte          `json:"-"`
}

// auditHash: sha256(prev ‖ поля через 0x1f). Время — в микросекундах UTC,
// как его хранит Postgres, иначе проверка не сойдётся.
func auditHash(prev []byte, e AuditEntry) []byte {
	var b bytes.Buffer
	b.Write(prev)
	for _, f := range []string{strconv.FormatInt(e.ActorID, 10), e.ActorRole, e.Action, e.Target, string(e.Details),
		strconv.Itoa(e.Status), e.IP, e.UserAgent, e.RequestID, e.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z")} {
		b.WriteByte(0x1f)
		b.WriteString(f)
	}
	h := sha256.Sum256(b.Bytes())
	return h[:]
}

// canonicalJSON — details после круга через jsonb (Postgres переупорядочивает
// ключи и пробелы), чтобы хеш считался от того же, что хранится.
func canonicalJSON(ctx context.Context, tx pgx.Tx, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, nil
	}
	var out string
	err := tx.QueryRow(ctx, `SELECT $1::jsonb::text`, string(raw)).Scan(&out)
	return json.RawMessage(out), err
}

// AppendAudit дописывает запись в цепочку. Блокировка — чтобы два запроса
// не взяли один и тот же «предыдущий» хеш.
func (s *Store) AppendAudit(ctx context.Context, e AuditEntry) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7324002)`); err != nil {
			return err
		}
		var prev []byte
		err := tx.QueryRow(ctx, `SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prev)
		if errors.Is(err, pgx.ErrNoRows) {
			prev = make([]byte, 32)
		} else if err != nil {
			return err
		}
		if e.Details, err = canonicalJSON(ctx, tx, e.Details); err != nil {
			return err
		}
		e.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
		hash := auditHash(prev, e)
		var ip *string
		if e.IP != "" {
			ip = &e.IP
		}
		var details *string
		if e.Details != nil {
			d := string(e.Details)
			details = &d
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO audit_log (actor_id, actor_role, action, target, details, status, ip, user_agent, request_id, created_at, prev_hash, hash)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7::inet, $8, $9, $10, $11, $12)`,
			e.ActorID, e.ActorRole, e.Action, e.Target, details, e.Status, ip, truncate(e.UserAgent, 300),
			e.RequestID, e.CreatedAt, prev, hash)
		return err
	})
}

const auditCols = `id, actor_id, actor_role, action, target, details::text, status, COALESCE(host(ip), ''), user_agent, request_id, created_at, prev_hash, hash`

func scanAudit(rows pgx.Rows) ([]AuditEntry, error) {
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var details *string
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorRole, &e.Action, &e.Target, &details, &e.Status, &e.IP,
			&e.UserAgent, &e.RequestID, &e.CreatedAt, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		if details != nil {
			e.Details = json.RawMessage(*details)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) AuditLog(ctx context.Context, actor, before int64, limit int) ([]AuditEntry, error) {
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.pool.Query(ctx, `SELECT `+auditCols+` FROM audit_log
		WHERE id < $1 AND ($2 = 0 OR actor_id = $2) ORDER BY id DESC LIMIT $3`, before, actor, limit)
	if err != nil {
		return nil, err
	}
	return scanAudit(rows)
}

// VerifyAudit проходит цепочку целиком. brokenAt — id первой записи,
// чей хеш или ссылка на предыдущую не сходятся (0 — всё цело).
func (s *Store) VerifyAudit(ctx context.Context) (checked int, brokenAt int64, err error) {
	prev := make([]byte, 32)
	after := int64(0)
	for {
		rows, err := s.pool.Query(ctx, `SELECT `+auditCols+` FROM audit_log WHERE id > $1 ORDER BY id LIMIT 1000`, after)
		if err != nil {
			return checked, 0, err
		}
		batch, err := scanAudit(rows)
		if err != nil {
			return checked, 0, err
		}
		if len(batch) == 0 {
			return checked, 0, nil
		}
		for _, e := range batch {
			if !bytes.Equal(e.PrevHash, prev) || !bytes.Equal(auditHash(prev, e), e.Hash) {
				return checked, e.ID, nil
			}
			prev, after = e.Hash, e.ID
			checked++
		}
	}
}

// ---------- тревоги ----------

type Alert struct {
	ID             int64           `json:"id"`
	Rule           string          `json:"rule"`
	Subject        string          `json:"subject"`
	Severity       string          `json:"severity"`
	Details        json.RawMessage `json:"details"`
	CreatedAt      time.Time       `json:"created_at"`
	AcknowledgedBy *int64          `json:"acknowledged_by"`
}

// Rule — правило детектора: SQL, который вставляет тревоги и возвращает новые.
// $1 — начало окна (для дедупликации).
type Rule struct {
	Name   string
	Window time.Duration
	SQL    string
}

// RunRules выполняет правила под advisory lock: если API запущено в
// нескольких экземплярах, детектор работает только в одном.
func (s *Store) RunRules(ctx context.Context, rules []Rule) ([]Alert, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(7324003)`).Scan(&locked); err != nil || !locked {
		return nil, err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7324003)`) //nolint:errcheck
	var out []Alert
	for _, r := range rules {
		window := time.Now().UTC().Truncate(r.Window)
		rows, err := conn.Query(ctx, r.SQL, window)
		if err != nil {
			return out, errors.New(r.Name + ": " + err.Error())
		}
		for rows.Next() {
			var a Alert
			if err := rows.Scan(&a.ID, &a.Rule, &a.Subject, &a.Severity, &a.Details, &a.CreatedAt); err != nil {
				rows.Close()
				return out, err
			}
			out = append(out, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *Store) Alerts(ctx context.Context, open bool, before int64, limit int) ([]Alert, error) {
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule, subject, severity, details, created_at, acknowledged_by FROM security_alerts
		WHERE id < $1 AND (NOT $2 OR acknowledged_at IS NULL) ORDER BY id DESC LIMIT $3`, before, open, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.Rule, &a.Subject, &a.Severity, &a.Details, &a.CreatedAt, &a.AcknowledgedBy); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AckAlert(ctx context.Context, id, by int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE security_alerts SET acknowledged_by = $2, acknowledged_at = now() WHERE id = $1 AND acknowledged_at IS NULL`, id, by)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// QueueDepth — для метрик: сколько ждёт в очередях.
func (s *Store) QueueDepth(ctx context.Context) (map[string]float64, error) {
	var jobs, callbacks, alerts float64
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM jobs WHERE status = 'queued'),
		       (SELECT count(*) FROM callback_deliveries WHERE status = 'queued'),
		       (SELECT count(*) FROM security_alerts WHERE acknowledged_at IS NULL)`).Scan(&jobs, &callbacks, &alerts)
	return map[string]float64{"jobs": jobs, "callbacks": callbacks, "open_alerts": alerts}, err
}

// PoolStats — состояние пула соединений с базой.
func (s *Store) PoolStats() map[string]float64 {
	st := s.pool.Stat()
	return map[string]float64{"acquired": float64(st.AcquiredConns()), "idle": float64(st.IdleConns()),
		"total": float64(st.TotalConns()), "max": float64(st.MaxConns())}
}

func (s *Store) RecordBackup(ctx context.Context, name string, size int64, sha string, encrypted, restoreOK bool, errMsg string, d time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backup_runs (name, size, sha256, encrypted, restore_ok, error, duration_ms) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		name, size, sha, encrypted, restoreOK, truncate(errMsg, 1000), d.Milliseconds())
	return err
}

// BackupStatus — время последней удачной копии (с проверкой восстановлением) и итог последней.
func (s *Store) BackupStatus(ctx context.Context) map[string]float64 {
	var lastOK, lastRun *time.Time
	var lastRestore *bool
	_ = s.pool.QueryRow(ctx, `
		SELECT (SELECT max(created_at) FROM backup_runs WHERE restore_ok),
		       (SELECT created_at FROM backup_runs ORDER BY id DESC LIMIT 1),
		       (SELECT restore_ok FROM backup_runs ORDER BY id DESC LIMIT 1)`).Scan(&lastOK, &lastRun, &lastRestore)
	out := map[string]float64{"last_success_timestamp": 0, "last_run_timestamp": 0, "last_restore_ok": 0}
	if lastOK != nil {
		out["last_success_timestamp"] = float64(lastOK.Unix())
	}
	if lastRun != nil {
		out["last_run_timestamp"] = float64(lastRun.Unix())
	}
	if lastRestore != nil && *lastRestore {
		out["last_restore_ok"] = 1
	}
	return out
}
