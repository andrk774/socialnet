package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Antenna struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Keywords  []string  `json:"keywords"`
	Excludes  []string  `json:"excludes"`
	Unread    int       `json:"unread"` // новых записей с прошлого просмотра (до 99)
	CreatedAt time.Time `json:"created_at"`
}

const MaxAntennas, MaxAntennasPlus = 5, 20

// orQuery — строка для websearch_to_tsquery: слова через «or». Слова уже
// проверены (буквы, цифры, пробел, дефис), кавычек в них нет; разбор делает
// PostgreSQL — подставить оператор нельзя.
func orQuery(words []string) string {
	parts := make([]string, 0, len(words))
	for _, k := range words {
		parts = append(parts, `"`+k+`"`)
	}
	return strings.Join(parts, " or ")
}

// antennaMatch: $2 — любое из слов, $3 — ни одного из исключений. Запросы
// склеиваются операторами tsquery (у websearch «-» связывается сильнее «or»).
const antennaMatch = ` to_tsvector('russian', p.text) @@ CASE WHEN $3 = '' THEN websearch_to_tsquery('russian', $2)
	ELSE websearch_to_tsquery('russian', $2) && !! websearch_to_tsquery('russian', $3) END`

func (s *Store) Antennas(ctx context.Context, user int64) ([]Antenna, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, keywords, excludes, last_seen_id, created_at FROM antennas WHERE user_id = $1 ORDER BY id`, user)
	if err != nil {
		return nil, err
	}
	type row struct {
		Antenna
		last int64
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Name, &r.Keywords, &r.Excludes, &r.last, &r.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Antenna, 0, len(list))
	for _, r := range list {
		// счётчик новых: те же правила видимости, что и в ленте антенны
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM posts p JOIN users a ON a.id = p.author_id
			LEFT JOIN communities c ON c.id = p.community_id
			WHERE p.id > $4 AND p.author_id <> $1 AND`+antennaMatch+` AND `+postVisible+` LIMIT 99) x`,
			user, orQuery(r.Keywords), orQuery(r.Excludes), r.last).Scan(&r.Unread); err != nil {
			return nil, err
		}
		out = append(out, r.Antenna)
	}
	return out, nil
}

func (s *Store) Antenna(ctx context.Context, user, id int64) (*Antenna, error) {
	var a Antenna
	err := s.pool.QueryRow(ctx, `SELECT id, name, keywords, excludes, created_at FROM antennas WHERE id = $1 AND user_id = $2`,
		id, user).Scan(&a.ID, &a.Name, &a.Keywords, &a.Excludes, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

func (s *Store) SaveAntenna(ctx context.Context, user, id int64, name string, keywords, excludes []string, max int) (int64, error) {
	if id > 0 {
		tag, err := s.pool.Exec(ctx, `UPDATE antennas SET name = $3, keywords = $4, excludes = $5 WHERE id = $1 AND user_id = $2`,
			id, user, name, keywords, excludes)
		if err == nil && tag.RowsAffected() == 0 {
			err = ErrNotFound
		}
		return id, err
	}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, user); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM antennas WHERE user_id = $1`, user).Scan(&n); err != nil {
			return err
		}
		if n >= max {
			return ErrTooMany
		}
		return tx.QueryRow(ctx, `INSERT INTO antennas (user_id, name, keywords, excludes) VALUES ($1, $2, $3, $4) RETURNING id`,
			user, name, keywords, excludes).Scan(&id)
	})
	return id, err
}

func (s *Store) DeleteAntenna(ctx context.Context, user, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM antennas WHERE id = $1 AND user_id = $2`, id, user)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// AntennaPosts — записи антенны (новые сверху); первая страница отмечает их просмотренными.
func (s *Store) AntennaPosts(ctx context.Context, user int64, a *Antenna, before int64, limit int) ([]Post, error) {
	posts, err := s.queryPosts(ctx, postSelect+`
		WHERE p.id < $4 AND`+antennaMatch+` AND `+postVisible+`
		ORDER BY p.id DESC LIMIT $5`, user, orQuery(a.Keywords), orQuery(a.Excludes), before, limit)
	if err != nil || len(posts) == 0 {
		return posts, err
	}
	_, err = s.pool.Exec(ctx, `UPDATE antennas SET last_seen_id = GREATEST(last_seen_id, $3) WHERE id = $1 AND user_id = $2`,
		a.ID, user, posts[0].ID)
	return posts, err
}
