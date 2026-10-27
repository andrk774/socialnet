package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type RoutePoint struct {
	ID         int64     `json:"id"`
	RouteID    string    `json:"route_id"`
	Author     UserCard  `json:"author"`
	Kind       string    `json:"kind"`
	Lat        float64   `json:"lat"`
	Lon        float64   `json:"lon"`
	Title      string    `json:"title"`
	Note       string    `json:"note"`
	PhotoURL   *string   `json:"photo_url"`
	ThumbURL   *string   `json:"thumb_url"`
	VerifiedOn string    `json:"verified_on"` // ГГГГ-ММ-ДД
	Checks     int       `json:"checks"`      // сколько человек подтвердили
	MyCheck    bool      `json:"my_check"`
	CreatedAt  time.Time `json:"created_at"`
}

type NewRoutePoint struct {
	RouteID, Kind, Title, Note string
	AuthorID                   int64
	Lat, Lon                   float64
	PhotoID                    *string
}

const MaxPointsPerDay = 30

const pointSelect = `
	SELECT p.id, p.route_id::text, ` + cardCols + `, p.kind, p.lat, p.lon, p.title, p.note, p.photo_id::text,
	       p.verified_on::text, (SELECT count(*) FROM route_point_checks c WHERE c.point_id = p.id),
	       EXISTS (SELECT 1 FROM route_point_checks c WHERE c.point_id = p.id AND c.user_id = $1), p.created_at
	FROM route_points p JOIN users u ON u.id = p.author_id
	WHERE p.hidden_at IS NULL AND u.banned_at IS NULL`

func (s *Store) queryPoints(ctx context.Context, sql string, args ...any) ([]RoutePoint, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoutePoint{}
	for rows.Next() {
		var p RoutePoint
		var avatar, photo *string
		if err := rows.Scan(&p.ID, &p.RouteID, &p.Author.ID, &p.Author.Username, &p.Author.FirstName, &p.Author.LastName, &avatar,
			&p.Kind, &p.Lat, &p.Lon, &p.Title, &p.Note, &photo, &p.VerifiedOn, &p.Checks, &p.MyCheck, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Author.AvatarURL = MediaURL(avatar, "thumb")
		p.PhotoURL, p.ThumbURL = MediaURL(photo, "full"), MediaURL(photo, "thumb")
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) RoutePoints(ctx context.Context, viewer int64, route string) ([]RoutePoint, error) {
	return s.queryPoints(ctx, pointSelect+` AND p.route_id = $2::uuid ORDER BY p.id`, viewer, route)
}

func (s *Store) RoutePoint(ctx context.Context, viewer, id int64) (*RoutePoint, error) {
	list, err := s.queryPoints(ctx, pointSelect+` AND p.id = $2`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

var ErrBadPhoto = errors.New("photo not found")

// AddRoutePoint: фото — только своё; не больше MaxPointsPerDay точек в сутки.
func (s *Store) AddRoutePoint(ctx context.Context, p NewRoutePoint) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, p.AuthorID); err != nil {
			return err
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM route_points WHERE author_id = $1 AND created_at > now() - interval '1 day'`,
			p.AuthorID).Scan(&today); err != nil {
			return err
		}
		if today >= MaxPointsPerDay {
			return ErrRateLimited
		}
		if p.PhotoID != nil {
			var mine bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM media WHERE id = $1::uuid AND owner_id = $2)`,
				*p.PhotoID, p.AuthorID).Scan(&mine); err != nil {
				return err
			}
			if !mine {
				return ErrBadPhoto
			}
		}
		return tx.QueryRow(ctx, `
			INSERT INTO route_points (route_id, author_id, kind, lat, lon, title, note, photo_id)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::uuid) RETURNING id`,
			p.RouteID, p.AuthorID, p.Kind, p.Lat, p.Lon, p.Title, p.Note, p.PhotoID).Scan(&id)
	})
	return id, err
}

// CheckRoutePoint: «на месте» — сегодняшняя дата становится датой проверки.
func (s *Store) CheckRoutePoint(ctx context.Context, id, user int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO route_point_checks (point_id, user_id) VALUES ($1, $2)
			ON CONFLICT (point_id, user_id) DO UPDATE SET checked_on = current_date`, id, user); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE route_points SET verified_on = current_date WHERE id = $1`, id)
		return err
	})
}

// DeleteRoutePoint: автор точки или владелец маршрута.
func (s *Store) DeleteRoutePoint(ctx context.Context, id, user int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM route_points p USING routes r
		WHERE p.id = $1 AND r.id = p.route_id AND (p.author_id = $2 OR r.owner_id = $2)`, id, user)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// RoutePointRoute — маршрут точки (для проверки доступа).
func (s *Store) RoutePointRoute(ctx context.Context, id int64) (string, error) {
	var r string
	err := s.pool.QueryRow(ctx, `SELECT route_id::text FROM route_points WHERE id = $1 AND hidden_at IS NULL`, id).Scan(&r)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return r, err
}
