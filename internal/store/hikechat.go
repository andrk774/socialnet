package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// HikeChatSync — что изменилось в чате похода после сверки с группой.
type HikeChatSync struct {
	ChatID  int64
	Owner   int64 // организатор
	Created bool
	Added   []int64
	Removed []int64
}

// SyncHikeChat приводит чат похода к списку группы: организатор и все со
// статусом going. Чат создаётся при первой сверке; название — как у похода.
// Для обычных мероприятий — пустой результат.
func (s *Store) SyncHikeChat(ctx context.Context, eventID int64) (*HikeChatSync, error) {
	res := &HikeChatSync{}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var kind, title string
		var creator int64
		err := tx.QueryRow(ctx, `SELECT kind, title, creator_id FROM events WHERE id = $1 FOR UPDATE`, eventID).Scan(&kind, &title, &creator)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil || kind != "hike" {
			return err
		}
		res.Owner = creator
		if r := []rune(title); len(r) > 100 {
			title = string(r[:100])
		}
		err = tx.QueryRow(ctx, `
			UPDATE conversations SET title = $2 WHERE event_id = $1 RETURNING id`, eventID, title).Scan(&res.ChatID)
		if errors.Is(err, pgx.ErrNoRows) {
			res.Created = true
			if err = tx.QueryRow(ctx, `
				INSERT INTO conversations (kind, title, owner_id, event_id) VALUES ('group', $1, $2, $3) RETURNING id`,
				title, creator, eventID).Scan(&res.ChatID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `
				INSERT INTO conversation_state (conversation_id, user_id, role) VALUES ($1, $2, 'owner')`, res.ChatID, creator); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		const want = `SELECT $2::bigint UNION SELECT user_id FROM event_attendees WHERE event_id = $3 AND status = 'going'`
		rows, err := tx.Query(ctx, `
			INSERT INTO conversation_state (conversation_id, user_id, last_read_id, last_delivered_id)
			SELECT $1, u.id, COALESCE(c.last_message_id, 0), COALESCE(c.last_message_id, 0)
			FROM (`+want+`) u(id) JOIN conversations c ON c.id = $1
			ON CONFLICT DO NOTHING RETURNING user_id`, res.ChatID, creator, eventID)
		if err != nil {
			return err
		}
		if res.Added, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `
			DELETE FROM conversation_state
			WHERE conversation_id = $1 AND user_id NOT IN (`+want+`) RETURNING user_id`, res.ChatID, creator, eventID)
		if err != nil {
			return err
		}
		res.Removed, err = pgx.CollectRows(rows, pgx.RowTo[int64])
		return err
	})
	return res, err
}

// HikeChatFor — id чата похода, если user в нём состоит.
func (s *Store) HikeChatFor(ctx context.Context, eventID, user int64) (*int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT c.id FROM conversations c
		JOIN conversation_state st ON st.conversation_id = c.id AND st.user_id = $2
		WHERE c.event_id = $1`, eventID, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}
