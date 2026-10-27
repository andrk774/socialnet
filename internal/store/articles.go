package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Article struct {
	ID          int64      `json:"id"`
	Author      UserCard   `json:"author"`
	Title       string     `json:"title"`
	Body        string     `json:"body,omitempty"` // исходник — в полной статье
	CoverURL    *string    `json:"cover_url"`
	CoverThumb  *string    `json:"cover_thumb_url"`
	CoverID     *string    `json:"cover_media_id"`
	RouteID     *string    `json:"route_id"`
	Status      string     `json:"status"` // draft | published
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at"`
}

type ArticleFields struct {
	Title, Body      string
	CoverID, RouteID *string
	Status           string
}

// articleVisible: черновик — только автору; опубликованное — всем, кроме
// заблокированных (в любую сторону); снятое модератором и от забаненных — никому.
const articleVisible = ` a.hidden_at IS NULL AND u.banned_at IS NULL
	AND (a.author_id = $1 OR (a.status = 'published'
	     AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = $1 AND b.blocked_id = a.author_id)
	                                              OR (b.blocker_id = a.author_id AND b.blocked_id = $1))))`

func (s *Store) queryArticles(ctx context.Context, withBody bool, sql string, args ...any) ([]Article, error) {
	body := `left(a.body, 2000)` // для списка хватит начала — на анонс
	if withBody {
		body = `a.body`
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id, `+cardCols+`, a.title, `+body+`, a.cover_media_id::text, a.route_id::text,
		a.status, a.created_at, a.updated_at, a.published_at
		FROM articles a JOIN users u ON u.id = a.author_id WHERE`+articleVisible+sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Article{}
	for rows.Next() {
		var a Article
		var avatar *string
		if err := rows.Scan(&a.ID, &a.Author.ID, &a.Author.Username, &a.Author.FirstName, &a.Author.LastName, &avatar,
			&a.Title, &a.Body, &a.CoverID, &a.RouteID, &a.Status, &a.CreatedAt, &a.UpdatedAt, &a.PublishedAt); err != nil {
			return nil, err
		}
		a.Author.AvatarURL = MediaURL(avatar, "thumb")
		a.CoverURL, a.CoverThumb = MediaURL(a.CoverID, "full"), MediaURL(a.CoverID, "thumb")
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Article(ctx context.Context, viewer, id int64) (*Article, error) {
	list, err := s.queryArticles(ctx, true, ` AND a.id = $2`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Articles — опубликованные, новые сверху; курсор — id.
func (s *Store) Articles(ctx context.Context, viewer, before int64, limit int) ([]Article, error) {
	return s.queryArticles(ctx, false, ` AND a.status = 'published' AND a.id < $2 ORDER BY a.id DESC LIMIT $3`, viewer, before, limit)
}

func (s *Store) MyArticles(ctx context.Context, me int64, limit int) ([]Article, error) {
	return s.queryArticles(ctx, false, ` AND a.author_id = $1 ORDER BY a.id DESC LIMIT $2`, me, limit)
}

// OwnedMedia — какие из фото принадлежат пользователю.
func (s *Store) OwnedMedia(ctx context.Context, owner int64, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM media WHERE owner_id = $1 AND id = ANY($2::uuid[])`, owner, ids)
	if err != nil {
		return nil, err
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	for _, id := range got {
		out[id] = true
	}
	return out, err
}

// Лимит статей в сутки: обычный и с «Плюсом».
const MaxArticlesPerDay, MaxArticlesPerDayPlus = 10, 30

func (s *Store) CreateArticle(ctx context.Context, author int64, f ArticleFields, perDay int) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, author); err != nil {
			return err
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM articles WHERE author_id = $1 AND created_at > now() - interval '1 day'`,
			author).Scan(&today); err != nil {
			return err
		}
		if today >= perDay {
			return ErrRateLimited
		}
		return tx.QueryRow(ctx, `
			INSERT INTO articles (author_id, title, body, cover_media_id, route_id, status, published_at)
			VALUES ($1, $2, $3, $4::uuid, $5::uuid, $6, CASE WHEN $6 = 'published' THEN now() END) RETURNING id`,
			author, f.Title, f.Body, f.CoverID, f.RouteID, f.Status).Scan(&id)
	})
	return id, err
}

func (s *Store) UpdateArticle(ctx context.Context, id, author int64, f ArticleFields) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE articles SET title = $3, body = $4, cover_media_id = $5::uuid, route_id = $6::uuid, status = $7,
		       updated_at = now(), published_at = CASE WHEN $7 = 'published' THEN COALESCE(published_at, now()) ELSE NULL END
		WHERE id = $1 AND author_id = $2 AND hidden_at IS NULL`, id, author, f.Title, f.Body, f.CoverID, f.RouteID, f.Status)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

func (s *Store) DeleteArticle(ctx context.Context, id, author int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM articles WHERE id = $1 AND author_id = $2`, id, author)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}
