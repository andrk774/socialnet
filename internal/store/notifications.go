package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Notification struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Actor     *UserCard `json:"actor,omitempty"`
	PostID    *int64    `json:"post_id,omitempty"`
	CommentID *int64    `json:"comment_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Read      bool      `json:"read"`
}

type NewNotification struct {
	UserID    int64
	Type      string
	ActorID   int64
	PostID    *int64
	CommentID *int64
}

// CreateNotification не уведомляет самого себя и тех, кто в блокировке
// (в любую сторону). Возвращает ErrNotFound, если уведомление не нужно.
func (s *Store) CreateNotification(ctx context.Context, n NewNotification) (*Notification, error) {
	var id int64
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, type, actor_id, post_id, comment_id)
		SELECT $1::bigint, $2::text, NULLIF($3::bigint, 0), $4::bigint, $5::bigint
		WHERE $1::bigint <> $3::bigint
		  -- пользователь отключил такой тип уведомлений
		  AND NOT EXISTS (SELECT 1 FROM notification_settings ns
			WHERE ns.user_id = $1::bigint AND $2::text = ANY(ns.muted_types))
		  AND NOT EXISTS (SELECT 1 FROM blocks b
			WHERE (b.blocker_id = $1 AND b.blocked_id = $3) OR (b.blocker_id = $3 AND b.blocked_id = $1))
		RETURNING id, created_at`,
		n.UserID, n.Type, n.ActorID, n.PostID, n.CommentID).Scan(&id, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	list, err := s.Notifications(ctx, n.UserID, id+1, 1)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func (s *Store) Notifications(ctx context.Context, userID, before int64, limit int) ([]Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT n.id, n.type, n.post_id, n.comment_id, n.created_at, n.read_at IS NOT NULL,
		       u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text
		FROM notifications n LEFT JOIN users u ON u.id = n.actor_id
		WHERE n.user_id = $1 AND n.id < $2
		ORDER BY n.id DESC LIMIT $3`, userID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var (
			n                    Notification
			aID                  *int64
			aUser, aFirst, aLast *string
			avatar               *string
		)
		if err := rows.Scan(&n.ID, &n.Type, &n.PostID, &n.CommentID, &n.CreatedAt, &n.Read,
			&aID, &aUser, &aFirst, &aLast, &avatar); err != nil {
			return nil, err
		}
		if aID != nil {
			n.Actor = &UserCard{ID: *aID, Username: *aUser, FirstName: *aFirst, LastName: *aLast,
				AvatarURL: MediaURL(avatar, "thumb")}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) UnreadNotifications(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// MarkNotificationsRead: пустой ids = все. user_id в условии не даёт
// пометить чужие уведомления, даже если подобрать их id.
func (s *Store) MarkNotificationsRead(ctx context.Context, userID int64, ids []int64) error {
	if len(ids) == 0 {
		_, err := s.pool.Exec(ctx,
			`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE user_id = $1 AND id = ANY($2) AND read_at IS NULL`, userID, ids)
	return err
}
