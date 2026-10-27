package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ConsentEntry struct {
	Document  string    `json:"document"`
	Version   string    `json:"version"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

// AcceptLegal записывает согласия в журнал и отмечает принятую редакцию.
func (s *Store) AcceptLegal(ctx context.Context, user int64, rev int, docs map[string]string, ip, ua string) error {
	if len(ua) > 300 {
		ua = ua[:300]
	}
	var ipArg *string
	if ip != "" {
		ipArg = &ip
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		for doc, ver := range docs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO consent_log (user_id, document, version, ip, user_agent) VALUES ($1, $2, $3, $4::inet, $5)`,
				user, doc, ver, ipArg, ua); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE users SET legal_rev = GREATEST(legal_rev, $2) WHERE id = $1`, user, rev)
		return err
	})
}

func (s *Store) LegalRev(ctx context.Context, user int64) (int, error) {
	var rev int
	err := s.pool.QueryRow(ctx, `SELECT legal_rev FROM users WHERE id = $1`, user).Scan(&rev)
	return rev, err
}

func (s *Store) ConsentLog(ctx context.Context, user int64) ([]ConsentEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT document, version, action, created_at FROM consent_log WHERE user_id = $1 ORDER BY id DESC LIMIT 200`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ConsentEntry])
}

// DeletedFiles — файлы удалённого аккаунта, которые нужно стереть из хранилища.
type DeletedFiles struct {
	Media, Documents, Routes, Videos []string
}

// DeleteAccount: отзыв согласия на обработку ПДн. Строка пользователя
// остаётся (на неё ссылаются платежи, которые закон велит хранить, и
// сообщения собеседников), но обезличивается: ни почты, ни имени, ни
// пароля. Всё, что публиковал пользователь, удаляется. Сообщения, уже
// отправленные другим, остаются у получателей от «удалённого аккаунта».
func (s *Store) DeleteAccount(ctx context.Context, id int64) (DeletedFiles, error) {
	var f DeletedFiles
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Свои сообщества переходят к старшему по роли и стажу участнику;
		// сообщество без участников удаляется.
		if _, err := tx.Exec(ctx, `
			WITH heir AS (
				SELECT DISTINCT ON (m.community_id) m.community_id, m.user_id
				FROM community_members m JOIN communities c ON c.id = m.community_id
				JOIN users u ON u.id = m.user_id
				WHERE c.owner_id = $1 AND m.user_id <> $1 AND u.banned_at IS NULL AND u.bot_community_id IS NULL
				ORDER BY m.community_id, CASE m.role WHEN 'admin' THEN 1 WHEN 'editor' THEN 2 ELSE 3 END, m.joined_at),
			moved AS (UPDATE communities c SET owner_id = heir.user_id FROM heir WHERE c.id = heir.community_id RETURNING c.id, c.owner_id)
			UPDATE community_members m SET role = 'owner' FROM moved WHERE m.community_id = moved.id AND m.user_id = moved.owner_id`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM communities WHERE owner_id = $1`, id); err != nil {
			return err
		}
		collect := func(q string, dst *[]string) error {
			rows, err := tx.Query(ctx, q, id)
			if err != nil {
				return err
			}
			*dst, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		}
		if err := collect(`DELETE FROM media WHERE owner_id = $1 RETURNING id::text`, &f.Media); err != nil {
			return err
		}
		if err := collect(`DELETE FROM documents WHERE owner_id = $1 RETURNING id::text`, &f.Documents); err != nil {
			return err
		}
		if err := collect(`DELETE FROM routes WHERE owner_id = $1 RETURNING id::text`, &f.Routes); err != nil {
			return err
		}
		if err := collect(`DELETE FROM videos WHERE owner_id = $1 RETURNING id::text`, &f.Videos); err != nil {
			return err
		}
		for _, q := range []string{
			`DELETE FROM posts WHERE author_id = $1`,
			`DELETE FROM comments WHERE author_id = $1`,
			`DELETE FROM stories WHERE author_id = $1`,
			`DELETE FROM follows WHERE follower_id = $1 OR followee_id = $1`,
			`DELETE FROM blocks WHERE blocker_id = $1 OR blocked_id = $1`,
			`DELETE FROM community_members WHERE user_id = $1`,
			`DELETE FROM community_join_requests WHERE user_id = $1`,
			`DELETE FROM event_attendees WHERE user_id = $1`,
			`DELETE FROM events WHERE creator_id = $1`,
			`DELETE FROM companion_ads WHERE author_id = $1`,
			`DELETE FROM hike_contacts WHERE user_id = $1`,
			`DELETE FROM route_points WHERE author_id = $1`,
			`DELETE FROM articles WHERE author_id = $1`,
			`DELETE FROM rides WHERE driver_id = $1`,
			`DELETE FROM antennas WHERE user_id = $1`,
			`DELETE FROM ride_requests WHERE user_id = $1`,
			`DELETE FROM route_point_checks WHERE user_id = $1`,
			`DELETE FROM bookmarks WHERE user_id = $1`,
			`DELETE FROM scheduled_posts WHERE author_id = $1`,
			`DELETE FROM notifications WHERE user_id = $1 OR actor_id = $1`,
			`DELETE FROM refresh_tokens WHERE user_id = $1`,
			`DELETE FROM api_tokens WHERE user_id = $1`,
			`DELETE FROM push_subscriptions WHERE user_id = $1`,
			`DELETE FROM user_keys WHERE user_id = $1`,
			`DELETE FROM backup_codes WHERE user_id = $1`,
			`DELETE FROM auth_tokens WHERE user_id = $1`,
			`DELETE FROM security_events WHERE user_id = $1`,
			`DELETE FROM notification_settings WHERE user_id = $1`,
			`DELETE FROM poll_votes WHERE user_id = $1`,
			`DELETE FROM post_likes WHERE user_id = $1`,
		} {
			if _, err := tx.Exec(ctx, q, id); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		_, err := tx.Exec(ctx, `
			UPDATE users SET
				email = 'deleted-' || id || '@deleted.invalid', username = 'deleted_' || id,
				first_name = 'Удалённый', last_name = 'аккаунт', password_hash = '!deleted',
				status_text = '', city = '', about = '', avatar_media_id = NULL, email_verified_at = NULL,
				totp_secret_enc = NULL, totp_enabled_at = NULL, totp_last_step = 0, last_seen_at = NULL,
				role = 'user', banned_at = COALESCE(banned_at, now()), ban_reason = 'account deleted',
				deleted_at = now(), updated_at = now()
			WHERE id = $1`, id)
		return err
	})
	return f, err
}
