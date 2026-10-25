package store

import (
	"context"
)

// Relationship описывает связь viewer → target.
type Relationship struct {
	IFollow   bool // я отправил заявку / подписан
	FollowsMe bool // он подписан на меня / прислал заявку
	IBlocked  bool
	BlockedMe bool
}

func (r Relationship) Friends() bool { return r.IFollow && r.FollowsMe }
func (r Relationship) Blocked() bool { return r.IBlocked || r.BlockedMe }

func (s *Store) Relationship(ctx context.Context, viewer, target int64) (Relationship, error) {
	var r Relationship
	err := s.pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM follows WHERE follower_id = $1 AND followee_id = $2),
			EXISTS (SELECT 1 FROM follows WHERE follower_id = $2 AND followee_id = $1),
			EXISTS (SELECT 1 FROM blocks  WHERE blocker_id  = $1 AND blocked_id  = $2),
			EXISTS (SELECT 1 FROM blocks  WHERE blocker_id  = $2 AND blocked_id  = $1)`,
		viewer, target).Scan(&r.IFollow, &r.FollowsMe, &r.IBlocked, &r.BlockedMe)
	return r, err
}

// Follow возвращает true, если подписка создана (а не уже была).
func (s *Store) Follow(ctx context.Context, follower, followee int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO follows (follower_id, followee_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, follower, followee)
	return tag.RowsAffected() == 1, err
}

func (s *Store) Unfollow(ctx context.Context, follower, followee int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2`, follower, followee)
	return err
}

// RejectFollower убирает чужую заявку (подписку на меня).
func (s *Store) RejectFollower(ctx context.Context, me, follower int64) error {
	return s.Unfollow(ctx, follower, me)
}

// Block разрывает подписки в обе стороны и запрещает дальнейшее общение.
func (s *Store) Block(ctx context.Context, blocker, blocked int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `
		INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		blocker, blocked); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM follows
		WHERE (follower_id = $1 AND followee_id = $2) OR (follower_id = $2 AND followee_id = $1)`,
		blocker, blocked); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Unblock(ctx context.Context, blocker, blocked int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`, blocker, blocked)
	return err
}

// UserCard — краткая карточка пользователя для списков.
type UserCard struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	AvatarURL *string `json:"avatar_url"`
}

// MediaURL — публичный адрес картинки. size: "full" или "thumb".
func MediaURL(id *string, size string) *string {
	if id == nil {
		return nil
	}
	u := "/media/" + *id + "/" + size
	return &u
}

const cardCols = `u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text`

func (s *Store) scanCards(ctx context.Context, sql string, args ...any) ([]UserCard, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserCard
	for rows.Next() {
		var c UserCard
		var avatar *string
		if err := rows.Scan(&c.ID, &c.Username, &c.FirstName, &c.LastName, &avatar); err != nil {
			return nil, err
		}
		c.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, c)
	}
	return out, rows.Err()
}

// Friends — взаимные подписки. Пагинация по id (after).
func (s *Store) Friends(ctx context.Context, userID, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+`
		FROM follows f1
		JOIN follows f2 ON f2.follower_id = f1.followee_id AND f2.followee_id = f1.follower_id
		JOIN users u ON u.id = f1.followee_id
		WHERE f1.follower_id = $1 AND u.id > $2 AND u.banned_at IS NULL
		ORDER BY u.id LIMIT $3`, userID, after, limit)
}

// IncomingRequests — на меня подписаны, а я нет.
func (s *Store) IncomingRequests(ctx context.Context, userID, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+`
		FROM follows f JOIN users u ON u.id = f.follower_id
		WHERE f.followee_id = $1 AND u.id > $2 AND u.banned_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM follows b WHERE b.follower_id = $1 AND b.followee_id = f.follower_id)
		ORDER BY u.id LIMIT $3`, userID, after, limit)
}

// OutgoingRequests — я подписан, а мне не ответили.
func (s *Store) OutgoingRequests(ctx context.Context, userID, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+`
		FROM follows f JOIN users u ON u.id = f.followee_id
		WHERE f.follower_id = $1 AND u.id > $2 AND u.banned_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM follows b WHERE b.follower_id = f.followee_id AND b.followee_id = $1)
		ORDER BY u.id LIMIT $3`, userID, after, limit)
}

func (s *Store) BlockedUsers(ctx context.Context, userID, after int64, limit int) ([]UserCard, error) {
	return s.scanCards(ctx, `
		SELECT `+cardCols+`
		FROM blocks b JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1 AND u.id > $2
		ORDER BY u.id LIMIT $3`, userID, after, limit)
}

// UsersByUsernames — для упоминаний @username.
func (s *Store) UsersByUsernames(ctx context.Context, names []string) ([]UserCard, error) {
	if len(names) == 0 {
		return nil, nil
	}
	return s.scanCards(ctx, `
		SELECT `+cardCols+` FROM users u
		WHERE u.username = ANY($1::citext[]) AND u.banned_at IS NULL`, names)
}
