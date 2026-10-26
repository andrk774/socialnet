package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ScheduledPost — пост, ждущий своего времени. Payload — тело запроса
// POST /posts в том виде, в каком его проверил сервер.
type ScheduledPost struct {
	ID          int64           `json:"id"`
	AuthorID    int64           `json:"author_id"`
	CommunityID *int64          `json:"community_id,omitempty"`
	PublishAt   time.Time       `json:"publish_at"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	PostID      *int64          `json:"post_id,omitempty"`
	Error       string          `json:"error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

const scheduledCols = `id, author_id, community_id, publish_at, payload, status, post_id, error, created_at`

func scanScheduled(rows pgx.Rows) ([]ScheduledPost, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ScheduledPost, error) {
		var p ScheduledPost
		return p, r.Scan(&p.ID, &p.AuthorID, &p.CommunityID, &p.PublishAt, &p.Payload, &p.Status, &p.PostID, &p.Error, &p.CreatedAt)
	})
}

var ErrTooManyScheduled = errors.New("too many scheduled posts")

// MaxScheduled — сколько отложенных постов может ждать у одного автора.
const MaxScheduled = 100

func (s *Store) CreateScheduled(ctx context.Context, author int64, community *int64, at time.Time, payload []byte) (*ScheduledPost, error) {
	var out *ScheduledPost
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// блокировка строки автора — чтобы параллельные запросы не обошли лимит
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, author); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM scheduled_posts WHERE author_id = $1 AND status = 'pending'`,
			author).Scan(&n); err != nil {
			return err
		}
		if n >= MaxScheduled {
			return ErrTooManyScheduled
		}
		rows, err := tx.Query(ctx, `
			INSERT INTO scheduled_posts (author_id, community_id, publish_at, payload) VALUES ($1, $2, $3, $4)
			RETURNING `+scheduledCols, author, community, at, payload)
		if err != nil {
			return err
		}
		list, err := scanScheduled(rows)
		if err == nil && len(list) == 1 {
			out = &list[0]
		}
		return err
	})
	return out, err
}

// ScheduledList: свои отложенные посты, а с community — все отложенные посты
// сообщества (их видят редакторы).
func (s *Store) ScheduledList(ctx context.Context, author int64, community *int64) ([]ScheduledPost, error) {
	var rows pgx.Rows
	var err error
	if community != nil {
		rows, err = s.pool.Query(ctx, `SELECT `+scheduledCols+` FROM scheduled_posts
			WHERE community_id = $1 AND status IN ('pending', 'failed') ORDER BY publish_at LIMIT 200`, *community)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT `+scheduledCols+` FROM scheduled_posts
			WHERE author_id = $1 AND community_id IS NULL AND status IN ('pending', 'failed') ORDER BY publish_at LIMIT 200`, author)
	}
	if err != nil {
		return nil, err
	}
	return scanScheduled(rows)
}

func (s *Store) Scheduled(ctx context.Context, id int64) (*ScheduledPost, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+scheduledCols+` FROM scheduled_posts WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanScheduled(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Reschedule переносит время (только ждущие и неудавшиеся — их можно повторить).
func (s *Store) Reschedule(ctx context.Context, id int64, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE scheduled_posts SET publish_at = $2, status = 'pending', error = '', updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'failed')`, id, at)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotPending
	}
	return err
}

func (s *Store) CancelScheduled(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE scheduled_posts SET status = 'canceled', updated_at = now() WHERE id = $1 AND status IN ('pending', 'failed')`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotPending
	}
	return err
}

// DueScheduled — посты, время которых пришло. Публикация сама отметит пост
// в своей транзакции, поэтому два экземпляра API не опубликуют его дважды.
func (s *Store) DueScheduled(ctx context.Context, limit int) ([]ScheduledPost, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+scheduledCols+` FROM scheduled_posts
		WHERE status = 'pending' AND publish_at <= now() ORDER BY publish_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return scanScheduled(rows)
}

func (s *Store) FailScheduled(ctx context.Context, id int64, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE scheduled_posts SET status = 'failed', error = $2, updated_at = now() WHERE id = $1 AND status = 'pending'`, id, msg)
	return err
}
