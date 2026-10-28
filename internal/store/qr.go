package store

import (
	"context"
	"time"
)

// День считаем по Екатеринбургу: «сегодня» для походов на Урале.
const qrDay = `(now() AT TIME ZONE 'Asia/Yekaterinburg')::date`

// RecordQRScan: +1 скан за сегодня; false — такого мероприятия нет.
func (s *Store) RecordQRScan(ctx context.Context, eventID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO event_qr_stats (event_id, day, scans)
		SELECT id, `+qrDay+`, 1 FROM events WHERE id = $1
		ON CONFLICT (event_id, day) DO UPDATE SET scans = event_qr_stats.scans + 1`, eventID)
	return tag.RowsAffected() == 1, err
}

// RecordQRJoin: +1 запись в поход после перехода по QR.
func (s *Store) RecordQRJoin(ctx context.Context, eventID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO event_qr_stats (event_id, day, joins) VALUES ($1, `+qrDay+`, 1)
		ON CONFLICT (event_id, day) DO UPDATE SET joins = event_qr_stats.joins + 1`, eventID)
	return err
}

type QRDay struct {
	Day   string `json:"day"`
	Scans int    `json:"scans"`
	Joins int    `json:"joins"`
}

type QRStats struct {
	Scans     int     `json:"scans"`
	Joins     int     `json:"joins"`
	ScansWeek int     `json:"scans_week"`
	Days      []QRDay `json:"days"` // последние 30 дней, где что-то было
}

func (s *Store) QRStats(ctx context.Context, eventID int64) (*QRStats, error) {
	st := &QRStats{Days: []QRDay{}}
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(scans), 0), COALESCE(sum(joins), 0),
		       COALESCE(sum(scans) FILTER (WHERE day > `+qrDay+` - 7), 0)
		FROM event_qr_stats WHERE event_id = $1`, eventID).Scan(&st.Scans, &st.Joins, &st.ScansWeek)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT day, scans, joins FROM event_qr_stats
		WHERE event_id = $1 AND day > `+qrDay+` - 30 ORDER BY day`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d QRDay
		var day time.Time
		if err := rows.Scan(&day, &d.Scans, &d.Joins); err != nil {
			return nil, err
		}
		d.Day = day.Format(time.DateOnly)
		st.Days = append(st.Days, d)
	}
	return st, rows.Err()
}
