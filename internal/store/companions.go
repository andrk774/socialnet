package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CompanionAd — объявление «Ищу попутчиков».
type CompanionAd struct {
	ID         int64     `json:"id"`
	Author     UserCard  `json:"author"`
	Region     string    `json:"region"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	DateFrom   string    `json:"date_from"` // ГГГГ-ММ-ДД
	DateTo     string    `json:"date_to"`
	Difficulty *int      `json:"difficulty"`
	Seats      int       `json:"seats"`
	RouteID    *string   `json:"route_id"`
	Closed     bool      `json:"closed"`
	CreatedAt  time.Time `json:"created_at"`
}

type NewCompanionAd struct {
	AuthorID   int64
	Region     string
	Title      string
	Body       string
	DateFrom   time.Time
	DateTo     time.Time
	Difficulty *int
	Seats      int
	RouteID    *string
}

// CompanionFilter: пустые поля — без ограничения.
type CompanionFilter struct {
	Region        string
	From, To      *time.Time
	MaxDifficulty int
}

const companionSelect = `
	SELECT a.id, ` + cardCols + `, a.region, a.title, a.body, a.date_from::text, a.date_to::text,
	       a.difficulty, a.seats, a.route_id::text, a.closed_at IS NOT NULL, a.created_at
	FROM companion_ads a JOIN users u ON u.id = a.author_id
	WHERE a.hidden_at IS NULL AND u.banned_at IS NULL
	  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = $1 AND b.blocked_id = a.author_id)
	                                           OR (b.blocker_id = a.author_id AND b.blocked_id = $1))`

func (s *Store) queryCompanions(ctx context.Context, sql string, args ...any) ([]CompanionAd, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CompanionAd{}
	for rows.Next() {
		var a CompanionAd
		var avatar *string
		var diff *int16
		var seats int16
		if err := rows.Scan(&a.ID, &a.Author.ID, &a.Author.Username, &a.Author.FirstName, &a.Author.LastName, &avatar,
			&a.Region, &a.Title, &a.Body, &a.DateFrom, &a.DateTo, &diff, &seats, &a.RouteID, &a.Closed, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Author.AvatarURL = MediaURL(avatar, "thumb")
		a.Seats = int(seats)
		if diff != nil {
			d := int(*diff)
			a.Difficulty = &d
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

var ErrTooMany = errors.New("too many")

// MaxOpenCompanionAds — открытых объявлений у одного автора одновременно.
const MaxOpenCompanionAds = 5

func (s *Store) CreateCompanionAd(ctx context.Context, a NewCompanionAd) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// блокировка автора: два одновременных запроса не обойдут лимит
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, a.AuthorID); err != nil {
			return err
		}
		var open int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM companion_ads
			WHERE author_id = $1 AND closed_at IS NULL AND date_to >= current_date`, a.AuthorID).Scan(&open); err != nil {
			return err
		}
		if open >= MaxOpenCompanionAds {
			return ErrTooMany
		}
		return tx.QueryRow(ctx, `
			INSERT INTO companion_ads (author_id, region, title, body, date_from, date_to, difficulty, seats, route_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::uuid) RETURNING id`,
			a.AuthorID, a.Region, a.Title, a.Body, a.DateFrom, a.DateTo, a.Difficulty, a.Seats, a.RouteID).Scan(&id)
	})
	return id, err
}

func (s *Store) CompanionAd(ctx context.Context, viewer, id int64) (*CompanionAd, error) {
	list, err := s.queryCompanions(ctx, companionSelect+` AND a.id = $2`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// CompanionAds: открытые объявления, которые ещё не прошли, — ближайшие первыми.
func (s *Store) CompanionAds(ctx context.Context, viewer int64, f CompanionFilter, limit int) ([]CompanionAd, error) {
	var region *string
	if f.Region != "" {
		p := likePattern(f.Region)
		region = &p
	}
	return s.queryCompanions(ctx, companionSelect+`
		AND a.closed_at IS NULL AND a.date_to >= current_date
		AND ($2::text IS NULL OR lower(a.region) LIKE $2)
		AND ($3::date IS NULL OR a.date_to >= $3)
		AND ($4::date IS NULL OR a.date_from <= $4)
		AND ($5 = 0 OR a.difficulty IS NULL OR a.difficulty <= $5)
		ORDER BY a.date_from, a.id LIMIT $6`, viewer, region, f.From, f.To, f.MaxDifficulty, limit)
}

func (s *Store) MyCompanionAds(ctx context.Context, me int64, limit int) ([]CompanionAd, error) {
	return s.queryCompanions(ctx, companionSelect+` AND a.author_id = $1 ORDER BY a.id DESC LIMIT $2`, me, limit)
}

// CloseCompanionAd: автор закрывает (нашёл попутчиков) или открывает снова.
func (s *Store) CloseCompanionAd(ctx context.Context, id, author int64, closed bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE companion_ads SET closed_at = CASE WHEN $3 THEN COALESCE(closed_at, now()) ELSE NULL END
		WHERE id = $1 AND author_id = $2`, id, author, closed)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

func (s *Store) DeleteCompanionAd(ctx context.Context, id, author int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM companion_ads WHERE id = $1 AND author_id = $2`, id, author)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}
