package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPollClosed   = errors.New("poll is closed")
	ErrAlreadyVoted = errors.New("already voted")
	ErrBadOption    = errors.New("unknown poll option")
	ErrNotVoted     = errors.New("not voted")
	ErrAnonymous    = errors.New("poll is anonymous")
)

func createPoll(ctx context.Context, tx pgx.Tx, postID int64, p *NewPoll) error {
	var pollID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO polls (post_id, question, multiple, anonymous, closes_at) VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, postID, p.Question, p.Multiple, p.Anonymous, p.ClosesAt).Scan(&pollID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO poll_options (poll_id, position, text)
		SELECT $1, t.ord::smallint, t.text FROM unnest($2::text[]) WITH ORDINALITY AS t(text, ord)`,
		pollID, p.Options)
	return err
}

// decodePoll: число голосов по вариантам видно тому, кто уже проголосовал,
// автору и всем после закрытия опроса — как во ВКонтакте.
func decodePoll(raw []byte, isAuthor bool) (*Poll, error) {
	var p Poll
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.MyVotes == nil {
		p.MyVotes = []int64{}
	}
	if len(p.MyVotes) == 0 && !p.Closed && !isAuthor {
		p.ResultsHidden = true
		for i := range p.Options {
			p.Options[i].Votes = nil
		}
	}
	return &p, nil
}

// Vote: голос принимается один раз (переголосовать — через Retract).
// Строка опроса блокируется, поэтому счётчики сходятся при одновременных голосах.
func (s *Store) Vote(ctx context.Context, postID, user int64, options []int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var pollID int64
		var multiple, closed bool
		err := tx.QueryRow(ctx, `
			SELECT id, multiple, closes_at IS NOT NULL AND closes_at <= now()
			FROM polls WHERE post_id = $1 FOR UPDATE`, postID).Scan(&pollID, &multiple, &closed)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if closed {
			return ErrPollClosed
		}
		if len(options) == 0 || (!multiple && len(options) > 1) {
			return ErrBadOption
		}
		var voted bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM poll_votes WHERE poll_id = $1 AND user_id = $2)`,
			pollID, user).Scan(&voted); err != nil {
			return err
		}
		if voted {
			return ErrAlreadyVoted
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO poll_votes (poll_id, option_id, user_id)
			SELECT $1, o.id, $3 FROM poll_options o WHERE o.poll_id = $1 AND o.id = ANY($2::bigint[])`,
			pollID, options, user)
		if err != nil {
			return err
		}
		if int(tag.RowsAffected()) != len(options) {
			return ErrBadOption // вариант из чужого опроса или несуществующий
		}
		if _, err := tx.Exec(ctx, `UPDATE poll_options SET votes_count = votes_count + 1 WHERE poll_id = $1 AND id = ANY($2::bigint[])`,
			pollID, options); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE polls SET voters_count = voters_count + 1 WHERE id = $1`, pollID)
		return err
	})
}

// Retract отзывает голос (пока опрос открыт).
func (s *Store) Retract(ctx context.Context, postID, user int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var pollID int64
		var closed bool
		err := tx.QueryRow(ctx, `
			SELECT id, closes_at IS NOT NULL AND closes_at <= now() FROM polls WHERE post_id = $1 FOR UPDATE`,
			postID).Scan(&pollID, &closed)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if closed {
			return ErrPollClosed
		}
		rows, err := tx.Query(ctx, `DELETE FROM poll_votes WHERE poll_id = $1 AND user_id = $2 RETURNING option_id`, pollID, user)
		if err != nil {
			return err
		}
		removed, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return err
		}
		if len(removed) == 0 {
			return ErrNotVoted
		}
		if _, err := tx.Exec(ctx, `UPDATE poll_options SET votes_count = votes_count - 1 WHERE poll_id = $1 AND id = ANY($2::bigint[])`,
			pollID, removed); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE polls SET voters_count = voters_count - 1 WHERE id = $1`, pollID)
		return err
	})
}

// PollVoters — кто выбрал вариант. Для анонимных опросов — ErrAnonymous.
func (s *Store) PollVoters(ctx context.Context, postID, optionID, after int64, limit int) ([]UserCard, error) {
	var anonymous bool
	err := s.pool.QueryRow(ctx, `
		SELECT pl.anonymous FROM polls pl JOIN poll_options o ON o.poll_id = pl.id
		WHERE pl.post_id = $1 AND o.id = $2`, postID, optionID).Scan(&anonymous)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBadOption
	}
	if err != nil {
		return nil, err
	}
	if anonymous {
		return nil, ErrAnonymous
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text
		FROM poll_votes v JOIN users u ON u.id = v.user_id
		WHERE v.option_id = $1 AND u.banned_at IS NULL AND v.user_id > $2
		ORDER BY v.user_id LIMIT $3`, optionID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserCard{}
	for rows.Next() {
		var v UserCard
		var avatar *string
		if err := rows.Scan(&v.ID, &v.Username, &v.FirstName, &v.LastName, &avatar); err != nil {
			return nil, err
		}
		v.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, v)
	}
	return out, rows.Err()
}
