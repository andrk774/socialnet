package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AreaNotice — предупреждение на территории (прямоугольник и даты).
type AreaNotice struct {
	ID        int64      `json:"id"`
	Kind      string     `json:"kind"` // fire | closure | flood | other
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	SourceURL string     `json:"source_url"`
	BBox      [4]float64 `json:"bbox"` // min_lat, min_lon, max_lat, max_lon
	StartsOn  string     `json:"starts_on"`
	EndsOn    *string    `json:"ends_on"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type AreaNoticeFields struct {
	Kind, Title, Body, SourceURL string
	BBox                         [4]float64
	StartsOn                     time.Time
	EndsOn                       *time.Time
}

const noticeCols = `id, kind, title, body, source_url, min_lat, min_lon, max_lat, max_lon,
	to_char(starts_on, 'YYYY-MM-DD'), to_char(ends_on, 'YYYY-MM-DD'), updated_at`

func scanNotices(rows pgx.Rows, err error) ([]AreaNotice, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AreaNotice{}
	for rows.Next() {
		var n AreaNotice
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &n.SourceURL, &n.BBox[0], &n.BBox[1], &n.BBox[2], &n.BBox[3],
			&n.StartsOn, &n.EndsOn, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CreateAreaNotice(ctx context.Context, by int64, f AreaNoticeFields) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO area_notices (kind, title, body, source_url, min_lat, min_lon, max_lat, max_lon, starts_on, ends_on, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		f.Kind, f.Title, f.Body, f.SourceURL, f.BBox[0], f.BBox[1], f.BBox[2], f.BBox[3], f.StartsOn, f.EndsOn, by).Scan(&id)
	return id, err
}

func (s *Store) UpdateAreaNotice(ctx context.Context, id int64, f AreaNoticeFields) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE area_notices SET kind = $2, title = $3, body = $4, source_url = $5, min_lat = $6, min_lon = $7,
		       max_lat = $8, max_lon = $9, starts_on = $10, ends_on = $11, updated_at = now()
		WHERE id = $1`, id, f.Kind, f.Title, f.Body, f.SourceURL, f.BBox[0], f.BBox[1], f.BBox[2], f.BBox[3], f.StartsOn, f.EndsOn)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteAreaNotice(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM area_notices WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) AreaNotice(ctx context.Context, id int64) (*AreaNotice, error) {
	list, err := scanNotices(s.pool.Query(ctx, `SELECT `+noticeCols+` FROM area_notices WHERE id = $1`, id))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// AreaNotices — действующие на from..to (или все, включая прошедшие, при all).
func (s *Store) AreaNotices(ctx context.Context, all bool) ([]AreaNotice, error) {
	return scanNotices(s.pool.Query(ctx, `SELECT `+noticeCols+` FROM area_notices
		WHERE $1 OR ends_on IS NULL OR ends_on >= current_date
		ORDER BY starts_on DESC, id DESC LIMIT 200`, all))
}

// RouteBBox — прямоугольник маршрута.
func (s *Store) RouteBBox(ctx context.Context, id string) ([4]float64, error) {
	var b [4]float64
	err := s.pool.QueryRow(ctx, `SELECT min_lat, min_lon, max_lat, max_lon FROM routes WHERE id = $1::uuid`, id).Scan(&b[0], &b[1], &b[2], &b[3])
	return b, err
}

// NoticesFor — предупреждения, чей прямоугольник пересекает bbox и чьи даты
// пересекают from..to.
func (s *Store) NoticesFor(ctx context.Context, bbox [4]float64, from, to time.Time) ([]AreaNotice, error) {
	return scanNotices(s.pool.Query(ctx, `SELECT `+noticeCols+` FROM area_notices
		WHERE min_lat <= $3 AND max_lat >= $1 AND min_lon <= $4 AND max_lon >= $2
		  AND starts_on <= $6::date AND (ends_on IS NULL OR ends_on >= $5::date)
		ORDER BY kind = 'closure' DESC, kind = 'fire' DESC, starts_on DESC`,
		bbox[0], bbox[1], bbox[2], bbox[3], from.Format(time.DateOnly), to.Format(time.DateOnly))) // даты — в поясе from/to
}
