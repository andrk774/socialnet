package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Review struct {
	ID        int64     `json:"id"`
	Author    UserCard  `json:"author"`
	Kind      string    `json:"kind"` // organizer | route
	EventID   *int64    `json:"event_id,omitempty"`
	Event     string    `json:"event_title,omitempty"`
	RouteID   *string   `json:"route_id,omitempty"`
	Rating    int       `json:"rating"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Rating struct {
	Avg   float64 `json:"avg"`
	Count int     `json:"count"`
}

// Отзыв об организаторе можно оставить в течение этого срока после похода.
const ReviewWindow = 90 * 24 * time.Hour

var (
	ErrReviewTooEarly = errors.New("the hike has not ended yet")
	ErrReviewTooLate  = errors.New("the review window has closed")
	ErrCannotReview   = errors.New("only participants can review, and not their own")
)

// ReviewOrganizer — отзыв участника о походе и его организаторе (создать
// или исправить). Участник — тот, кто был в группе (going), не организатор.
func (s *Store) ReviewOrganizer(ctx context.Context, eventID, author int64, rating int, text string) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var creator int64
		var ended time.Time
		var kind, status string
		err := tx.QueryRow(ctx, `
			SELECT e.kind, e.creator_id, COALESCE(e.ends_at, e.starts_at + interval '6 hours'), COALESCE(a.status, '')
			FROM events e LEFT JOIN event_attendees a ON a.event_id = e.id AND a.user_id = $2
			WHERE e.id = $1`, eventID, author).Scan(&kind, &creator, &ended, &status)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != "hike") {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case creator == author || status != "going":
			return ErrCannotReview
		case time.Now().Before(ended):
			return ErrReviewTooEarly
		case time.Since(ended) > ReviewWindow:
			return ErrReviewTooLate
		}
		return tx.QueryRow(ctx, `
			INSERT INTO reviews (author_id, kind, event_id, organizer_id, rating, text)
			VALUES ($1, 'organizer', $2, $3, $4, $5)
			ON CONFLICT (event_id, author_id) WHERE kind = 'organizer' AND event_id IS NOT NULL
			DO UPDATE SET rating = EXCLUDED.rating, text = EXCLUDED.text, updated_at = now()
			RETURNING id`, author, eventID, creator, rating, text).Scan(&id)
	})
	return id, err
}

// ReviewRoute — отзыв о маршруте каталога (не своём).
func (s *Store) ReviewRoute(ctx context.Context, routeID string, author int64, rating int, text string) (int64, error) {
	var owner int64
	err := s.pool.QueryRow(ctx, `
		SELECT r.owner_id FROM routes r JOIN users u ON u.id = r.owner_id WHERE`+publicWhere+` AND r.id = $1::uuid`, routeID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if owner == author {
		return 0, ErrCannotReview
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO reviews (author_id, kind, route_id, rating, text) VALUES ($1, 'route', $2::uuid, $3, $4)
		ON CONFLICT (route_id, author_id) WHERE kind = 'route'
		DO UPDATE SET rating = EXCLUDED.rating, text = EXCLUDED.text, updated_at = now()
		RETURNING id`, author, routeID, rating, text).Scan(&id)
	return id, err
}

func (s *Store) DeleteMyReview(ctx context.Context, author int64, eventID *int64, routeID *string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM reviews WHERE author_id = $1
		  AND (($2::bigint IS NOT NULL AND kind = 'organizer' AND event_id = $2) OR ($3::uuid IS NOT NULL AND kind = 'route' AND route_id = $3::uuid))`,
		author, eventID, routeID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

const reviewVisible = ` rv.hidden_at IS NULL AND u.banned_at IS NULL`

func (s *Store) reviews(ctx context.Context, where string, args ...any) ([]Review, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT rv.id, `+cardCols+`, rv.kind, rv.event_id, COALESCE(e.title, ''), rv.route_id::text, rv.rating, rv.text, rv.created_at, rv.updated_at
		FROM reviews rv JOIN users u ON u.id = rv.author_id LEFT JOIN events e ON e.id = rv.event_id
		WHERE`+reviewVisible+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		var r Review
		var av *string
		if err := rows.Scan(&r.ID, &r.Author.ID, &r.Author.Username, &r.Author.FirstName, &r.Author.LastName, &av,
			&r.Kind, &r.EventID, &r.Event, &r.RouteID, &r.Rating, &r.Text, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Author.AvatarURL = MediaURL(av, "thumb")
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) rating(ctx context.Context, where string, args ...any) (Rating, error) {
	var r Rating
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(round(avg(rv.rating)::numeric, 1), 0)::float8, count(*)
		FROM reviews rv JOIN users u ON u.id = rv.author_id WHERE`+reviewVisible+where, args...).Scan(&r.Avg, &r.Count)
	return r, err
}

func (s *Store) OrganizerReviews(ctx context.Context, organizer, before int64, limit int) (Rating, []Review, error) {
	r, err := s.rating(ctx, ` AND rv.kind = 'organizer' AND rv.organizer_id = $1`, organizer)
	if err != nil {
		return r, nil, err
	}
	list, err := s.reviews(ctx, ` AND rv.kind = 'organizer' AND rv.organizer_id = $1 AND rv.id < $2 ORDER BY rv.id DESC LIMIT $3`, organizer, before, limit)
	return r, list, err
}

func (s *Store) RouteReviews(ctx context.Context, routeID string, before int64, limit int) (Rating, []Review, error) {
	r, err := s.rating(ctx, ` AND rv.kind = 'route' AND rv.route_id = $1::uuid`, routeID)
	if err != nil {
		return r, nil, err
	}
	list, err := s.reviews(ctx, ` AND rv.kind = 'route' AND rv.route_id = $1::uuid AND rv.id < $2 ORDER BY rv.id DESC LIMIT $3`, routeID, before, limit)
	return r, list, err
}

func (s *Store) RouteRating(ctx context.Context, routeID string) (Rating, error) {
	return s.rating(ctx, ` AND rv.kind = 'route' AND rv.route_id = $1::uuid`, routeID)
}

// MyEventReview — свой отзыв о походе (nil — не писал).
func (s *Store) MyEventReview(ctx context.Context, eventID, author int64) (*Review, error) {
	list, err := s.reviews(ctx, ` AND rv.kind = 'organizer' AND rv.event_id = $1 AND rv.author_id = $2`, eventID, author)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}
