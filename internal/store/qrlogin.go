package store

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
)

// Вход по QR-коду (см. миграцию 0029).

const QRLoginTTL = 2 * time.Minute

var (
	ErrQRExpired = errors.New("qr login expired or used")
	ErrQRPending = errors.New("qr login not approved yet")
	ErrQRDenied  = errors.New("qr login rejected")
)

type QRLogin struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Store) CreateQRLogin(ctx context.Context, id string, pollHash []byte, meta SessionMeta) (time.Time, error) {
	// заодно — уборка давно истёкших
	if _, err := s.pool.Exec(ctx, `DELETE FROM qr_logins WHERE expires_at < now() - interval '1 hour'`); err != nil {
		return time.Time{}, err
	}
	var exp time.Time
	ua := meta.UserAgent
	if len(ua) > 500 {
		ua = truncate(ua, 500)
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO qr_logins (id, poll_hash, user_agent, ip, expires_at) VALUES ($1, $2, $3, $4, now() + $5::interval)
		RETURNING expires_at`, id, pollHash, ua, meta.IP, QRLoginTTL.String()).Scan(&exp)
	return exp, err
}

// QRLoginInfo — что показать на телефоне перед подтверждением.
func (s *Store) QRLoginInfo(ctx context.Context, id string) (*QRLogin, error) {
	q := &QRLogin{ID: id}
	var ip *netip.Addr
	err := s.pool.QueryRow(ctx, `
		SELECT user_agent, ip, created_at, expires_at FROM qr_logins
		WHERE id = $1 AND expires_at > now() AND approved_at IS NULL AND rejected_at IS NULL AND consumed_at IS NULL`,
		id).Scan(&q.UserAgent, &ip, &q.CreatedAt, &q.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrQRExpired
	}
	if ip != nil {
		q.IP = ip.String()
	}
	return q, err
}

// DecideQRLogin: телефон подтверждает (approve) или отклоняет запрос.
func (s *Store) DecideQRLogin(ctx context.Context, id string, user int64, approve bool) error {
	q := `UPDATE qr_logins SET rejected_at = now() WHERE id = $1 AND $2::bigint > 0`
	if approve {
		q = `UPDATE qr_logins SET approved_by = $2, approved_at = now() WHERE id = $1`
	}
	tag, err := s.pool.Exec(ctx, q+` AND expires_at > now() AND approved_at IS NULL AND rejected_at IS NULL AND consumed_at IS NULL`, id, user)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrQRExpired
	}
	return err
}

// ClaimQRLogin: новое устройство с секретом опроса забирает подтверждённый
// вход — ровно один раз. Возвращает id пользователя.
func (s *Store) ClaimQRLogin(ctx context.Context, id string, pollHash []byte) (int64, error) {
	var user int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var approved *int64
		var rejected, expired, stale bool
		// подтверждённый вход можно забрать ещё минуту после срока кода
		err := tx.QueryRow(ctx, `
			SELECT approved_by, rejected_at IS NOT NULL, expires_at <= now(), expires_at <= now() - interval '1 minute'
			FROM qr_logins WHERE id = $1 AND poll_hash = $2 AND consumed_at IS NULL FOR UPDATE`, id, pollHash).
			Scan(&approved, &rejected, &expired, &stale)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrQRExpired
		case err != nil:
			return err
		case rejected:
			return ErrQRDenied
		case stale || (approved == nil && expired):
			return ErrQRExpired
		case approved == nil:
			return ErrQRPending
		}
		user = *approved
		_, err = tx.Exec(ctx, `UPDATE qr_logins SET consumed_at = now() WHERE id = $1`, id)
		return err
	})
	return user, err
}
