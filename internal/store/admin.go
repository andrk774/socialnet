package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Данные для администратора. Хэш пароля, секрет 2FA, резервные коды и
// токены сюда не попадают ни в каком виде — администратору они не нужны:
// для взлома достаточно утечки, а для помощи пользователю есть сброс по почте.

type AdminUser struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	AvatarURL     *string    `json:"avatar_url"`
	Role          string     `json:"role"`
	EmailVerified bool       `json:"email_verified"`
	BannedAt      *time.Time `json:"banned_at,omitempty"`
	BanReason     string     `json:"ban_reason,omitempty"`
	IsBot         bool       `json:"is_bot"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
}

const adminUserCols = `u.id, u.username, u.email, u.first_name, u.last_name, u.avatar_media_id::text, u.role,
	u.email_verified_at IS NOT NULL, u.banned_at, u.ban_reason, u.bot_community_id IS NOT NULL, u.created_at, u.last_seen_at`

func scanAdminUser(r pgx.CollectableRow) (AdminUser, error) {
	var u AdminUser
	var avatar *string
	err := r.Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName, &avatar, &u.Role,
		&u.EmailVerified, &u.BannedAt, &u.BanReason, &u.IsBot, &u.CreatedAt, &u.LastSeenAt)
	u.AvatarURL = MediaURL(avatar, "thumb")
	return u, err
}

// AdminSearchUsers: q — число (id), email, username или имя; filter —
// all | banned | staff | unverified. Курсор — id по убыванию.
func (s *Store) AdminSearchUsers(ctx context.Context, q, filter string, before int64, limit int) ([]AdminUser, error) {
	if before <= 0 {
		before = 1<<63 - 1
	}
	where := []string{"u.id < $1"}
	args := []any{before}
	q = strings.TrimSpace(q)
	if q != "" {
		if id, err := strconv.ParseInt(q, 10, 64); err == nil {
			args = append(args, id)
			where = append(where, "u.id = $"+strconv.Itoa(len(args)))
		} else {
			args = append(args, likePattern(q))
			n := "$" + strconv.Itoa(len(args))
			where = append(where, "(lower(u.email::text) LIKE "+n+
				" OR lower(u.username::text || ' ' || u.first_name || ' ' || u.last_name) LIKE "+n+")")
		}
	}
	switch filter {
	case "banned":
		where = append(where, "u.banned_at IS NOT NULL")
	case "staff":
		where = append(where, "u.role <> 'user'")
	case "unverified":
		where = append(where, "u.email_verified_at IS NULL")
	}
	args = append(args, limit)
	rows, err := s.pool.Query(ctx, `SELECT `+adminUserCols+` FROM users u WHERE `+strings.Join(where, " AND ")+
		` ORDER BY u.id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanAdminUser)
}

type AdminUserCard struct {
	AdminUser
	TwoFactor      bool            `json:"two_factor_enabled"`
	Sessions       int             `json:"active_sessions"`
	Posts          int             `json:"posts"`
	Comments       int             `json:"comments"`
	Friends        int             `json:"friends"`
	ReportsAgainst int             `json:"open_reports_against"`
	ReportsFiled   int             `json:"reports_filed"`
	Balance        int64           `json:"balance"`
	Debt           int64           `json:"debt"`
	LastLoginAt    *time.Time      `json:"last_login_at,omitempty"`
	Events         []SecurityEvent `json:"recent_security_events"`
}

func (s *Store) AdminUserCard(ctx context.Context, id int64) (*AdminUserCard, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+adminUserCols+` FROM users u WHERE u.id = $1`, id)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, scanAdminUser)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	c := &AdminUserCard{AdminUser: list[0]}
	err = s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT totp_enabled_at IS NOT NULL FROM users WHERE id = $1),
		  (SELECT count(DISTINCT session_id) FROM refresh_tokens
		     WHERE user_id = $1 AND revoked_at IS NULL AND replaced_at IS NULL AND expires_at > now()),
		  (SELECT count(*) FROM posts WHERE author_id = $1 AND deleted_at IS NULL),
		  (SELECT count(*) FROM comments WHERE author_id = $1 AND deleted_at IS NULL),
		  (SELECT count(*) FROM follows x WHERE x.follower_id = $1
		     AND EXISTS (SELECT 1 FROM follows y WHERE y.follower_id = x.followee_id AND y.followee_id = $1)),
		  (SELECT count(*) FROM reports r WHERE r.status = 'open' AND (
		       (r.target_type = 'user' AND r.target_id = $1)
		    OR (r.target_type = 'post' AND r.target_id IN (SELECT id FROM posts WHERE author_id = $1))
		    OR (r.target_type = 'comment' AND r.target_id IN (SELECT id FROM comments WHERE author_id = $1)))),
		  (SELECT count(*) FROM reports WHERE reporter_id = $1),
		  COALESCE((SELECT balance FROM accounts WHERE kind = 'user' AND owner_id = $1), 0),
		  COALESCE((SELECT amount FROM wallet_debts WHERE user_id = $1), 0),
		  (SELECT max(created_at) FROM security_events WHERE user_id = $1 AND type = 'login')`, id).
		Scan(&c.TwoFactor, &c.Sessions, &c.Posts, &c.Comments, &c.Friends, &c.ReportsAgainst, &c.ReportsFiled,
			&c.Balance, &c.Debt, &c.LastLoginAt)
	if err != nil {
		return nil, err
	}
	if c.Events, err = s.SecurityEvents(ctx, id, 1<<63-1, 20); err != nil {
		return nil, err
	}
	return c, nil
}

type AdminStats struct {
	Users           int   `json:"users"`
	UsersNew24h     int   `json:"users_new_24h"`
	UsersNew7d      int   `json:"users_new_7d"`
	Active24h       int   `json:"active_24h"`
	Banned          int   `json:"banned"`
	Posts24h        int   `json:"posts_24h"`
	Comments24h     int   `json:"comments_24h"`
	Messages24h     int   `json:"messages_24h"`
	OpenReports     int   `json:"open_reports"`
	OpenAlerts      int   `json:"open_alerts"`
	NewVulnReports  int   `json:"new_vulnerability_reports"`
	ScheduledQueued int   `json:"scheduled_posts_pending"`
	Documents       int   `json:"documents"`
	PaidKopecks30d  int64 `json:"paid_kopecks_30d"`
}

func (s *Store) AdminStats(ctx context.Context) (*AdminStats, error) {
	var st AdminStats
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM users WHERE bot_community_id IS NULL),
		  (SELECT count(*) FROM users WHERE bot_community_id IS NULL AND created_at > now() - interval '24 hours'),
		  (SELECT count(*) FROM users WHERE bot_community_id IS NULL AND created_at > now() - interval '7 days'),
		  (SELECT count(*) FROM users WHERE last_seen_at > now() - interval '24 hours'),
		  (SELECT count(*) FROM users WHERE banned_at IS NOT NULL),
		  (SELECT count(*) FROM posts WHERE created_at > now() - interval '24 hours'),
		  (SELECT count(*) FROM comments WHERE created_at > now() - interval '24 hours'),
		  (SELECT count(*) FROM messages WHERE created_at > now() - interval '24 hours'),
		  (SELECT count(*) FROM reports WHERE status = 'open'),
		  (SELECT count(*) FROM security_alerts WHERE acknowledged_at IS NULL),
		  (SELECT count(*) FROM vuln_reports WHERE status = 'new'),
		  (SELECT count(*) FROM scheduled_posts WHERE status = 'pending'),
		  (SELECT count(*) FROM documents WHERE deleted_at IS NULL),
		  COALESCE((SELECT sum(amount_kopecks - refunded_kopecks) FROM payments
		            WHERE status IN ('succeeded', 'partially_refunded') AND created_at > now() - interval '30 days'), 0)`).
		Scan(&st.Users, &st.UsersNew24h, &st.UsersNew7d, &st.Active24h, &st.Banned, &st.Posts24h, &st.Comments24h,
			&st.Messages24h, &st.OpenReports, &st.OpenAlerts, &st.NewVulnReports, &st.ScheduledQueued, &st.Documents,
			&st.PaidKopecks30d)
	return &st, err
}

// RevokeUserEverywhere: все сеансы и токены приложений пользователя (админ «выкинуть отовсюду»).
func (s *Store) RevokeUserEverywhere(ctx context.Context, id int64) error {
	if err := s.RevokeAllSessions(ctx, id); err != nil {
		return err
	}
	err := s.RevokeUserOAuthTokens(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
