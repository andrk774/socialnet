package store

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- поиск ----------

// likePattern экранирует спецсимволы LIKE, чтобы запрос "%" не находил всех.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.ToLower(q)) + "%"
}

// SearchUsers ищет по имени и username (триграммный индекс pg_trgm).
// Не находит тех, кто скрылся из поиска, забанен или в блокировке.
func (s *Store) SearchUsers(ctx context.Context, viewer int64, q string, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+` FROM users u
		WHERE lower(u.username::text || ' ' || u.first_name || ' ' || u.last_name) LIKE $2
		  AND (u.searchable OR u.id = $1) AND u.banned_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM blocks b
			WHERE (b.blocker_id = u.id AND b.blocked_id = $1) OR (b.blocker_id = $1 AND b.blocked_id = u.id))
		ORDER BY similarity(lower(u.username::text || ' ' || u.first_name || ' ' || u.last_name), lower($3)) DESC, u.id
		LIMIT $4`, viewer, likePattern(q), q, limit)
}

func (s *Store) SearchCommunities(ctx context.Context, viewer int64, q string, limit int) ([]Community, error) {
	rows, err := s.pool.Query(ctx, communitySelect+`
		WHERE lower(c.slug::text || ' ' || c.name) LIKE $2 AND c.hidden_at IS NULL
		ORDER BY similarity(lower(c.slug::text || ' ' || c.name), lower($3)) DESC, c.id
		LIMIT $4`, viewer, likePattern(q), q, limit)
	if err != nil {
		return nil, err
	}
	return scanCommunities(rows)
}

// ---------- жалобы ----------

var ErrDuplicate = errors.New("duplicate")

// TargetAuthor: автор контента (для user — сам пользователь), ErrNotFound если нет.
func (s *Store) TargetAuthor(ctx context.Context, typ string, id int64) (int64, error) {
	var q string
	switch typ {
	case "post":
		q = `SELECT author_id FROM posts WHERE id = $1 AND deleted_at IS NULL`
	case "comment":
		q = `SELECT author_id FROM comments WHERE id = $1 AND deleted_at IS NULL`
	case "user":
		q = `SELECT id FROM users WHERE id = $1`
	case "community":
		q = `SELECT owner_id FROM communities WHERE id = $1`
	case "companion":
		q = `SELECT author_id FROM companion_ads WHERE id = $1 AND hidden_at IS NULL`
	case "route_point":
		q = `SELECT author_id FROM route_points WHERE id = $1 AND hidden_at IS NULL`
	case "article":
		q = `SELECT author_id FROM articles WHERE id = $1 AND hidden_at IS NULL AND status = 'published'`
	case "review":
		q = `SELECT author_id FROM reviews WHERE id = $1 AND hidden_at IS NULL`
	default:
		return 0, ErrNotFound
	}
	var author int64
	err := s.pool.QueryRow(ctx, q, id).Scan(&author)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return author, err
}

// TargetAuthorUUID — для треков и видео (у них id — UUID).
func (s *Store) TargetAuthorUUID(ctx context.Context, typ, id string) (int64, error) {
	var q string
	switch typ {
	case "track":
		q = `SELECT uploader_id FROM tracks WHERE id = $1::uuid AND removed_at IS NULL`
	case "video":
		q = `SELECT owner_id FROM videos WHERE id = $1::uuid AND status <> 'removed'`
	case "document":
		q = `SELECT owner_id FROM documents WHERE id = $1::uuid AND deleted_at IS NULL AND hidden_at IS NULL`
	case "route":
		q = `SELECT owner_id FROM routes WHERE id = $1::uuid AND deleted_at IS NULL AND hidden_at IS NULL`
	default:
		return 0, ErrNotFound
	}
	var author int64
	err := s.pool.QueryRow(ctx, q, id).Scan(&author)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return author, err
}

func (s *Store) CreateReport(ctx context.Context, reporter int64, typ string, target int64, targetUUID *string, reason string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO reports (reporter_id, target_type, target_id, target_uuid, reason) VALUES ($1, $2, $3, $4::uuid, $5)
		RETURNING id`, reporter, typ, target, targetUUID, reason).Scan(&id)
	if _, ok := isUnique(err); ok {
		return 0, ErrDuplicate
	}
	return id, err
}

type Report struct {
	ID         int64      `json:"id"`
	Reporter   UserCard   `json:"reporter"`
	TargetType string     `json:"target_type"`
	TargetID   int64      `json:"target_id"`
	TargetUUID *string    `json:"target_uuid,omitempty"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	HandledBy  *int64     `json:"handled_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	HandledAt  *time.Time `json:"handled_at,omitempty"`
	// Target — что именно обжаловано, чтобы модератор решал, не открывая
	// объект отдельно; nil — объект удалён.
	Target *ReportTarget `json:"target,omitempty"`
}

// ReportTarget — краткое содержимое объекта жалобы. Ref — куда вести
// ссылку: пост комментария, маршрут точки, адрес сообщества.
type ReportTarget struct {
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
	AuthorID int64  `json:"author_id"`
	Author   string `json:"author"` // username
	Hidden   bool   `json:"hidden"`
	Ref      string `json:"ref,omitempty"`
}

func (s *Store) Report(ctx context.Context, id int64) (*Report, error) {
	list, err := s.reports(ctx, `r.id = $1`, id, int64(0), 1)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

func (s *Store) Reports(ctx context.Context, status string, after int64, limit int) ([]Report, error) {
	return s.reports(ctx, `r.status = $1`, status, after, limit)
}

func (s *Store) reports(ctx context.Context, where string, arg any, after int64, limit int) ([]Report, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text,
		       r.target_type, r.target_id, r.target_uuid::text, r.reason, r.status, r.handled_by, r.created_at, r.handled_at,
		       t.title, t.snippet, t.author_id, t.hidden, t.ref, au.username
		FROM reports r JOIN users u ON u.id = r.reporter_id
		LEFT JOIN LATERAL (`+reportTargetSQL+`) t ON true
		LEFT JOIN users au ON au.id = t.author_id
		WHERE `+where+` AND r.id > $2 ORDER BY r.id LIMIT $3`, arg, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		var avatar, title, snippet, ref, authorName *string
		var author *int64
		var hidden *bool
		if err := rows.Scan(&r.ID, &r.Reporter.ID, &r.Reporter.Username, &r.Reporter.FirstName, &r.Reporter.LastName,
			&avatar, &r.TargetType, &r.TargetID, &r.TargetUUID, &r.Reason, &r.Status, &r.HandledBy, &r.CreatedAt, &r.HandledAt,
			&title, &snippet, &author, &hidden, &ref, &authorName); err != nil {
			return nil, err
		}
		if author != nil {
			r.Target = &ReportTarget{Title: *title, Snippet: *snippet, AuthorID: *author, Author: *authorName, Hidden: *hidden}
			if ref != nil {
				r.Target.Ref = *ref
			}
		}
		r.Reporter.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, r)
	}
	return out, rows.Err()
}

// reportTargetSQL — по одной ветке на тип жалобы; срабатывает ровно та,
// что совпала по target_type. Удалённые объекты (deleted_at) не находятся.
const reportTargetSQL = `
	SELECT ''::text AS title, left(p.text, 300) AS snippet, p.author_id, p.hidden_at IS NOT NULL AS hidden, NULL::text AS ref
	  FROM posts p WHERE r.target_type = 'post' AND p.id = r.target_id AND p.deleted_at IS NULL
	UNION ALL
	SELECT '', left(c.text, 300), c.author_id, c.hidden_at IS NOT NULL, c.post_id::text
	  FROM comments c WHERE r.target_type = 'comment' AND c.id = r.target_id AND c.deleted_at IS NULL
	UNION ALL
	SELECT cm.name, left(cm.description, 300), cm.owner_id, cm.hidden_at IS NOT NULL, cm.slug
	  FROM communities cm WHERE r.target_type = 'community' AND cm.id = r.target_id
	UNION ALL
	SELECT x.username, trim(x.first_name || ' ' || x.last_name), x.id, x.banned_at IS NOT NULL, NULL
	  FROM users x WHERE r.target_type = 'user' AND x.id = r.target_id
	UNION ALL
	SELECT a.title, a.region || ': ' || left(a.body, 300), a.author_id, a.hidden_at IS NOT NULL, NULL
	  FROM companion_ads a WHERE r.target_type = 'companion' AND a.id = r.target_id
	UNION ALL
	SELECT ar.title, left(ar.body, 300), ar.author_id, ar.hidden_at IS NOT NULL, ar.id::text
	  FROM articles ar WHERE r.target_type = 'article' AND ar.id = r.target_id
	UNION ALL
	SELECT rp.title, left(rp.note, 300), rp.author_id, rp.hidden_at IS NOT NULL, rp.route_id::text
	  FROM route_points rp WHERE r.target_type = 'route_point' AND rp.id = r.target_id
	UNION ALL
	SELECT rt.title, left(rt.public_note, 300), rt.owner_id, rt.hidden_at IS NOT NULL, rt.id::text
	  FROM routes rt WHERE r.target_type = 'route' AND rt.id = r.target_uuid AND rt.deleted_at IS NULL
	UNION ALL
	SELECT CASE rv.kind WHEN 'route' THEN 'Отзыв о маршруте' ELSE 'Отзыв об организаторе' END || ': ' || rv.rating || ' из 5',
	       left(rv.text, 300), rv.author_id, rv.hidden_at IS NOT NULL, COALESCE(rv.route_id::text, rv.event_id::text)
	  FROM reviews rv WHERE r.target_type = 'review' AND rv.id = r.target_id
	UNION ALL
	SELECT tr.title, tr.artist, tr.uploader_id, tr.removed_at IS NOT NULL, NULL
	  FROM tracks tr WHERE r.target_type = 'track' AND tr.id = r.target_uuid
	UNION ALL
	SELECT v.title, left(v.description, 300), v.owner_id, v.status = 'removed', NULL
	  FROM videos v WHERE r.target_type = 'video' AND v.id = r.target_uuid
	UNION ALL
	SELECT d.title || '.' || d.ext, '', d.owner_id, d.hidden_at IS NOT NULL, NULL
	  FROM documents d WHERE r.target_type = 'document' AND d.id = r.target_uuid AND d.deleted_at IS NULL
	LIMIT 1`

// CloseReports закрывает все открытые жалобы на объект одним решением.
func (s *Store) CloseReports(ctx context.Context, typ string, target int64, targetUUID *string, moderator int64, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE reports SET status = $4, handled_by = $3, handled_at = now()
		WHERE target_type = $1 AND target_id = $2 AND target_uuid IS NOT DISTINCT FROM $5::uuid
		  AND status = 'open'`, typ, target, moderator, status, targetUUID)
	return err
}

// ---------- действия модераторов ----------

// SetHidden скрывает или возвращает пост, комментарий или сообщество.
func (s *Store) SetHidden(ctx context.Context, typ string, id int64, hidden bool) error {
	var table string
	switch typ { // имя таблицы только из белого списка
	case "post":
		table = "posts"
	case "comment":
		table = "comments"
	case "community":
		table = "communities"
	case "companion":
		table = "companion_ads"
	case "route_point":
		table = "route_points"
	case "article":
		table = "articles"
	case "review":
		table = "reviews"
	default:
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE `+table+` SET hidden_at = CASE WHEN $2 THEN COALESCE(hidden_at, now()) ELSE NULL END
		WHERE id = $1`, id, hidden)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Ban блокирует аккаунт и завершает все его сеансы.
func (s *Store) Ban(ctx context.Context, userID int64, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE users SET banned_at = now(), ban_reason = $2 WHERE id = $1`, userID, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Unban(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET banned_at = NULL, ban_reason = '' WHERE id = $1`, userID)
	return err
}

func (s *Store) SetRole(ctx context.Context, userID int64, role string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, userID, role)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type ModAction struct {
	ID          int64     `json:"id"`
	ModeratorID int64     `json:"moderator_id"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    int64     `json:"target_id"`
	ReportID    *int64    `json:"report_id,omitempty"`
	TargetUUID  *string   `json:"target_uuid,omitempty"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) LogModeration(ctx context.Context, a ModAction) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO moderation_log (moderator_id, action, target_type, target_id, report_id, note, target_uuid)
		VALUES ($1, $2, $3, $4, $5, $6, $7::uuid)`, a.ModeratorID, a.Action, a.TargetType, a.TargetID, a.ReportID, a.Note, a.TargetUUID)
	return err
}

func (s *Store) ModerationLog(ctx context.Context, before int64, limit int) ([]ModAction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, moderator_id, action, target_type, target_id, report_id, target_uuid::text, note, created_at
		FROM moderation_log WHERE id < $1 ORDER BY id DESC LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModAction{}
	for rows.Next() {
		var a ModAction
		if err := rows.Scan(&a.ID, &a.ModeratorID, &a.Action, &a.TargetType, &a.TargetID, &a.ReportID, &a.TargetUUID, &a.Note, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------- сообщения об уязвимостях ----------

type VulnReport struct {
	ID          int64       `json:"id"`
	Contact     string      `json:"contact"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	IP          *netip.Addr `json:"ip"`
	Status      string      `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
}

func (s *Store) CreateVulnReport(ctx context.Context, contact, title, desc string, ip *netip.Addr) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO vuln_reports (contact, title, description, ip) VALUES ($1, $2, $3, $4) RETURNING id`,
		contact, title, desc, ip).Scan(&id)
	return id, err
}

func (s *Store) VulnReports(ctx context.Context, before int64, limit int) ([]VulnReport, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, contact, title, description, ip, status, created_at FROM vuln_reports
		WHERE id < $1 ORDER BY id DESC LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VulnReport{}
	for rows.Next() {
		var v VulnReport
		if err := rows.Scan(&v.ID, &v.Contact, &v.Title, &v.Description, &v.IP, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SetVulnStatus(ctx context.Context, id int64, status string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE vuln_reports SET status = $2 WHERE id = $1`, id, status)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
