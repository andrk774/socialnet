package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type Emoji struct {
	ID          int64     `json:"id"`
	CommunityID int64     `json:"community_id"`
	Shortcode   string    `json:"shortcode"`
	URL         string    `json:"url"`
	Reaction    string    `json:"reaction"` // значение для POST /posts/{id}/reactions
	CreatedAt   time.Time `json:"created_at"`
}

// Эмодзи в сообществе: обычно и если у владельца «Плюс» (клубы).
const MaxEmojiPerCommunity, MaxEmojiPerCommunityPlus = 50, 200

var ErrDuplicateEmoji = errors.New("shortcode taken")

func (s *Store) CommunityEmoji(ctx context.Context, community int64) ([]Emoji, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, community_id, shortcode, media_id::text, created_at
		FROM community_emoji WHERE community_id = $1 ORDER BY id`, community)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Emoji{}
	for rows.Next() {
		var e Emoji
		var media string
		if err := rows.Scan(&e.ID, &e.CommunityID, &e.Shortcode, &media, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.URL = "/media/" + media + "/thumb"
		e.Reaction = "e:" + strconv.FormatInt(e.ID, 10)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddEmoji: фото — только своё; не больше MaxEmojiPerCommunity.
func (s *Store) AddEmoji(ctx context.Context, community, by int64, shortcode, media string, max int) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM communities WHERE id = $1 FOR UPDATE`, community); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM community_emoji WHERE community_id = $1`, community).Scan(&n); err != nil {
			return err
		}
		if n >= max {
			return ErrTooMany
		}
		var mine bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM media WHERE id = $1::uuid AND owner_id = $2)`, media, by).Scan(&mine); err != nil {
			return err
		}
		if !mine {
			return ErrBadPhoto
		}
		err := tx.QueryRow(ctx, `INSERT INTO community_emoji (community_id, shortcode, media_id, created_by)
			VALUES ($1, $2, $3::uuid, $4) RETURNING id`, community, shortcode, media, by).Scan(&id)
		if _, dup := isUnique(err); dup {
			return ErrDuplicateEmoji
		}
		return err
	})
	return id, err
}

// DeleteEmoji: реакции этим эмодзи становятся обычным «нравится» — счётчик не меняется.
func (s *Store) DeleteEmoji(ctx context.Context, community, id int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM community_emoji WHERE id = $1 AND community_id = $2`, id, community)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `UPDATE post_likes SET reaction = 'like' WHERE reaction = $1`, "e:"+strconv.FormatInt(id, 10))
		return err
	})
}

// EmojiCommunity — сообщество эмодзи (для проверки реакции).
func (s *Store) EmojiCommunity(ctx context.Context, id int64) (int64, error) {
	var c int64
	err := s.pool.QueryRow(ctx, `SELECT community_id FROM community_emoji WHERE id = $1`, id).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return c, err
}
