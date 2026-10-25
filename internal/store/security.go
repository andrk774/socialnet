package store

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- сессии ----------

// StartSession создаёт сессию и возвращает её id.
func (s *Store) StartSession(ctx context.Context, userID int64, hash []byte, ttl time.Duration, meta SessionMeta) (string, error) {
	var sid string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, now() + $5::interval)
		RETURNING session_id::text`,
		userID, hash, truncate(meta.UserAgent, 512), meta.IP, ttl).Scan(&sid)
	return sid, err
}

// SessionActive: у сессии есть живой refresh-токен.
func (s *Store) SessionActive(ctx context.Context, userID int64, sid string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM refresh_tokens
			WHERE session_id = $2::uuid AND user_id = $1 AND revoked_at IS NULL AND expires_at > now())`,
		userID, sid).Scan(&ok)
	return ok, err
}

type Session struct {
	ID         string      `json:"id"`
	UserAgent  string      `json:"user_agent"`
	IP         *netip.Addr `json:"ip"`
	StartedAt  time.Time   `json:"started_at"`
	LastActive time.Time   `json:"last_active"`
	Current    bool        `json:"current"`
}

func (s *Store) Sessions(ctx context.Context, userID int64, currentSID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (session_id) session_id::text, user_agent, ip, session_started_at, created_at
		FROM refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY session_id, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.ID, &x.UserAgent, &x.IP, &x.StartedAt, &x.LastActive); err != nil {
			return nil, err
		}
		x.Current = x.ID == currentSID
		out = append(out, x)
	}
	return out, rows.Err()
}

// RevokeSession: только свою (user_id в условии — защита от IDOR).
func (s *Store) RevokeSession(ctx context.Context, userID int64, sid string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND session_id = $2::uuid AND revoked_at IS NULL`, userID, sid)
	return tag.RowsAffected() > 0, err
}

func (s *Store) RevokeOtherSessions(ctx context.Context, userID int64, keepSID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND session_id <> $2::uuid AND revoked_at IS NULL`, userID, keepSID)
	return err
}

// ---------- одноразовые токены из писем ----------

// CreateAuthToken гасит прежние неиспользованные токены того же назначения:
// действует только последняя ссылка.
func (s *Store) CreateAuthToken(ctx context.Context, userID int64, purpose string, hash []byte, ttl time.Duration) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
		UPDATE auth_tokens SET used_at = now()
		WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`, userID, purpose); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_tokens (user_id, purpose, token_hash, expires_at)
		VALUES ($1, $2, $3, now() + $4::interval)`, userID, purpose, hash, ttl); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ConsumeAuthToken атомарно помечает токен использованным. Повторно не сработает.
func (s *Store) ConsumeAuthToken(ctx context.Context, purpose string, hash []byte) (int64, error) {
	var userID int64
	err := s.pool.QueryRow(ctx, `
		UPDATE auth_tokens SET used_at = now()
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`, hash, purpose).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return userID, err
}

func (s *Store) MarkEmailVerified(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`, userID)
	return err
}

// ---------- 2FA ----------

// SetPendingTOTP сохраняет секрет до подтверждения кодом. Если 2FA уже
// включена — ErrConflict.
func (s *Store) SetPendingTOTP(ctx context.Context, userID int64, sealed []byte) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET totp_secret_enc = $2, totp_last_step = 0
		WHERE id = $1 AND totp_enabled_at IS NULL`, userID, sealed)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) EnableTOTP(ctx context.Context, userID, step int64, backupHashes [][]byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
		UPDATE users SET totp_enabled_at = now(), totp_last_step = $2 WHERE id = $1`, userID, step); err != nil {
		return err
	}
	if err := replaceBackupCodes(ctx, tx, userID, backupHashes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DisableTOTP(ctx context.Context, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
		UPDATE users SET totp_secret_enc = NULL, totp_enabled_at = NULL, totp_last_step = 0
		WHERE id = $1`, userID); err != nil {
		return err
	}
	if err := replaceBackupCodes(ctx, tx, userID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdvanceTOTPStep запоминает использованный шаг. Условие step > last не даёт
// двум параллельным запросам принять один и тот же код.
func (s *Store) AdvanceTOTPStep(ctx context.Context, userID, step int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET totp_last_step = $2 WHERE id = $1 AND totp_last_step < $2`, userID, step)
	return tag.RowsAffected() == 1, err
}

func replaceBackupCodes(ctx context.Context, tx pgx.Tx, userID int64, hashes [][]byte) error {
	if _, err := tx.Exec(ctx, `DELETE FROM backup_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if len(hashes) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO backup_codes (user_id, code_hash) SELECT $1, unnest($2::bytea[])`, userID, hashes)
	return err
}

func (s *Store) ReplaceBackupCodes(ctx context.Context, userID int64, hashes [][]byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := replaceBackupCodes(ctx, tx, userID, hashes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UseBackupCode: код одноразовый, гасится атомарно.
func (s *Store) UseBackupCode(ctx context.Context, userID int64, hash []byte) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE backup_codes SET used_at = now()
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, userID, hash)
	return tag.RowsAffected() == 1, err
}

func (s *Store) BackupCodesLeft(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM backup_codes WHERE user_id = $1 AND used_at IS NULL`, userID).Scan(&n)
	return n, err
}

// ---------- журнал безопасности ----------

type SecurityEvent struct {
	ID        int64       `json:"id"`
	Type      string      `json:"type"`
	IP        *netip.Addr `json:"ip"`
	UserAgent string      `json:"user_agent"`
	CreatedAt time.Time   `json:"created_at"`
}

func (s *Store) AddSecurityEvent(ctx context.Context, userID int64, typ string, meta SessionMeta) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO security_events (user_id, type, ip, user_agent) VALUES ($1, $2, $3, $4)`,
		userID, typ, meta.IP, truncate(meta.UserAgent, 512))
	return err
}

// KnownDevice: был ли уже успешный вход с этого браузера и IP.
func (s *Store) KnownDevice(ctx context.Context, userID int64, meta SessionMeta) (known, firstLogin bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM security_events
				WHERE user_id = $1 AND type IN ('login', 'login_new_device', 'register')
				  AND user_agent = $2 AND ip IS NOT DISTINCT FROM $3),
			NOT EXISTS (SELECT 1 FROM security_events
				WHERE user_id = $1 AND type IN ('login', 'login_new_device', 'register'))`,
		userID, truncate(meta.UserAgent, 512), meta.IP).Scan(&known, &firstLogin)
	return known, firstLogin, err
}

func (s *Store) SecurityEvents(ctx context.Context, userID, before int64, limit int) ([]SecurityEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, type, ip, user_agent, created_at FROM security_events
		WHERE user_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`, userID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SecurityEvent{}
	for rows.Next() {
		var e SecurityEvent
		if err := rows.Scan(&e.ID, &e.Type, &e.IP, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
