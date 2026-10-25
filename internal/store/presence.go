package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- онлайн-статус ----------

// TouchLastSeen обновляет «был в сети» не чаще раза в минуту,
// чтобы не писать в базу на каждый запрос.
func (s *Store) TouchLastSeen(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET last_seen_at = now()
		WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '60 seconds')`, userID)
	return err
}

type PresenceRow struct {
	UserID     int64
	LastSeen   *time.Time
	Visibility string
	Rel        Relationship
}

// PresenceRows: данные для онлайн-статуса нескольких пользователей сразу.
func (s *Store) PresenceRows(ctx context.Context, viewer int64, ids []int64) ([]PresenceRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.last_seen_at, u.online_visibility,
			EXISTS (SELECT 1 FROM follows WHERE follower_id = $1 AND followee_id = u.id),
			EXISTS (SELECT 1 FROM follows WHERE follower_id = u.id AND followee_id = $1),
			EXISTS (SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = u.id),
			EXISTS (SELECT 1 FROM blocks WHERE blocker_id = u.id AND blocked_id = $1)
		FROM users u WHERE u.id = ANY($2) AND u.banned_at IS NULL`, viewer, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PresenceRow
	for rows.Next() {
		var p PresenceRow
		if err := rows.Scan(&p.UserID, &p.LastSeen, &p.Visibility,
			&p.Rel.IFollow, &p.Rel.FollowsMe, &p.Rel.IBlocked, &p.Rel.BlockedMe); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------- настройки уведомлений ----------

type NotificationSettings struct {
	MutedTypes  []string `json:"muted_types"`
	EmailDigest bool     `json:"email_digest"`
	PushEnabled bool     `json:"push_enabled"`
	QuietStart  *int     `json:"quiet_start"` // минуты от полуночи, напр. 1380 = 23:00
	QuietEnd    *int     `json:"quiet_end"`
	Timezone    string   `json:"timezone"`
}

func DefaultNotificationSettings() NotificationSettings {
	return NotificationSettings{MutedTypes: []string{}, EmailDigest: true, PushEnabled: true, Timezone: "UTC"}
}

func (s *Store) NotificationSettings(ctx context.Context, userID int64) (NotificationSettings, error) {
	n := DefaultNotificationSettings()
	err := s.pool.QueryRow(ctx, `
		SELECT muted_types, email_digest, push_enabled, quiet_start, quiet_end, timezone
		FROM notification_settings WHERE user_id = $1`, userID).
		Scan(&n.MutedTypes, &n.EmailDigest, &n.PushEnabled, &n.QuietStart, &n.QuietEnd, &n.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultNotificationSettings(), nil
	}
	return n, err
}

func (s *Store) SaveNotificationSettings(ctx context.Context, userID int64, n NotificationSettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_settings (user_id, muted_types, email_digest, push_enabled, quiet_start, quiet_end, timezone)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			muted_types = EXCLUDED.muted_types, email_digest = EXCLUDED.email_digest,
			push_enabled = EXCLUDED.push_enabled, quiet_start = EXCLUDED.quiet_start,
			quiet_end = EXCLUDED.quiet_end, timezone = EXCLUDED.timezone`,
		userID, n.MutedTypes, n.EmailDigest, n.PushEnabled, n.QuietStart, n.QuietEnd, n.Timezone)
	return err
}

// ---------- подписки Web Push ----------

type PushSub struct {
	ID       int64
	Endpoint string
	P256dh   []byte
	Auth     []byte
}

// SavePushSub: чужую подписку (тот же endpoint у другого пользователя)
// перехватить нельзя — вернёт ErrConflict. Не больше 10 устройств.
func (s *Store) SavePushSub(ctx context.Context, userID int64, endpoint string, p256dh, auth []byte) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth) VALUES ($1, $2, $3, $4)
		ON CONFLICT (endpoint) DO UPDATE SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth
		WHERE push_subscriptions.user_id = EXCLUDED.user_id`, userID, endpoint, p256dh, auth)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	if n >= 10 { // удаляем самую старую
		_, err = s.pool.Exec(ctx, `
			DELETE FROM push_subscriptions WHERE id IN (
				SELECT id FROM push_subscriptions WHERE user_id = $1 ORDER BY id LIMIT $2)`, userID, n-9)
	}
	return err
}

func (s *Store) DeletePushSub(ctx context.Context, userID int64, endpoint string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	return err
}

func (s *Store) DeletePushSubByID(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id)
	return err
}

func (s *Store) PushSubs(ctx context.Context, userID int64) ([]PushSub, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSub
	for rows.Next() {
		var p PushSub
		if err := rows.Scan(&p.ID, &p.Endpoint, &p.P256dh, &p.Auth); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------- письма о непрочитанном ----------

type DigestCandidate struct {
	UserID    int64
	Email     string
	FirstName string
	Unread    int
}

// DigestCandidates: подтверждённый email, письмо не отключено, давно не заходил,
// есть непрочитанные уведомления старше часа, прошлое письмо — больше суток назад.
func (s *Store) DigestCandidates(ctx context.Context, limit int) ([]DigestCandidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.first_name,
		       (SELECT count(*) FROM notifications n WHERE n.user_id = u.id AND n.read_at IS NULL)
		FROM users u
		LEFT JOIN notification_settings ns ON ns.user_id = u.id
		WHERE u.email_verified_at IS NOT NULL AND u.banned_at IS NULL
		  AND COALESCE(ns.email_digest, true)
		  AND (u.last_seen_at IS NULL OR u.last_seen_at < now() - interval '1 hour')
		  AND (u.last_digest_at IS NULL OR u.last_digest_at < now() - interval '24 hours')
		  AND EXISTS (SELECT 1 FROM notifications n
		              WHERE n.user_id = u.id AND n.read_at IS NULL AND n.created_at < now() - interval '1 hour')
		ORDER BY u.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DigestCandidate
	for rows.Next() {
		var c DigestCandidate
		if err := rows.Scan(&c.UserID, &c.Email, &c.FirstName, &c.Unread); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ClaimDigest помечает письмо отправленным. Условие в UPDATE гарантирует,
// что при нескольких экземплярах API письмо уйдёт один раз.
func (s *Store) ClaimDigest(ctx context.Context, userID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET last_digest_at = now()
		WHERE id = $1 AND (last_digest_at IS NULL OR last_digest_at < now() - interval '24 hours')`, userID)
	return tag.RowsAffected() == 1, err
}
