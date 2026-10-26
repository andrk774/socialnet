package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Video struct {
	ID          string     `json:"id"`
	OwnerID     int64      `json:"owner_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"` // processing | ready | failed
	DurationSec *int       `json:"duration_sec,omitempty"`
	Width       *int       `json:"width,omitempty"`
	Height      *int       `json:"height,omitempty"`
	Qualities   []string   `json:"qualities"`
	PosterURL   string     `json:"poster_url,omitempty"`
	HLSURL      string     `json:"hls_url,omitempty"`
	MP4URL      string     `json:"mp4_url,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadyAt     *time.Time `json:"ready_at,omitempty"`
}

const videoCols = `id::text, owner_id, title, description, status, duration_sec, width, height, renditions, created_at, ready_at, mp4`

func scanVideos(rows pgx.Rows) ([]Video, error) {
	defer rows.Close()
	out := []Video{}
	for rows.Next() {
		var v Video
		var mp4 *string
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.Title, &v.Description, &v.Status, &v.DurationSec,
			&v.Width, &v.Height, &v.Qualities, &v.CreatedAt, &v.ReadyAt, &mp4); err != nil {
			return nil, err
		}
		if v.Status == "ready" {
			v.PosterURL, v.HLSURL = VideoURL(v.ID, "poster.jpg"), VideoURL(v.ID, "master.m3u8")
			if mp4 != nil {
				v.MP4URL = VideoURL(v.ID, *mp4+".mp4")
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type NewVideo struct {
	ID, Title, Description string
	OwnerID                int64
	SourceSHA              []byte
	Format                 string
}

// CreateVideo создаёт видео в статусе processing и ставит задачу на обработку —
// в одной транзакции, чтобы видео не «зависло» без задачи.
func (s *Store) CreateVideo(ctx context.Context, v NewVideo) error {
	payload, _ := json.Marshal(map[string]string{"video_id": v.ID, "format": v.Format})
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO videos (id, owner_id, title, description, source_sha256)
			VALUES ($1::uuid, $2, $3, $4, $5)`, v.ID, v.OwnerID, v.Title, v.Description, v.SourceSHA); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO jobs (kind, payload) VALUES ('video_transcode', $1)`, payload)
		return err
	})
}

func (s *Store) Video(ctx context.Context, id string) (*Video, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+videoCols+` FROM videos WHERE id = $1::uuid AND status <> 'removed'`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanVideos(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// UserVideos: чужие — только готовые, свои — все (видно ход обработки).
func (s *Store) UserVideos(ctx context.Context, owner int64, self bool, limit int) ([]Video, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+videoCols+` FROM videos
		WHERE owner_id = $1 AND (status = 'ready' OR ($2 AND status <> 'removed'))
		ORDER BY created_at DESC LIMIT $3`, owner, self, limit)
	if err != nil {
		return nil, err
	}
	return scanVideos(rows)
}

func (s *Store) UpdateVideo(ctx context.Context, id string, title, desc *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE videos SET title = COALESCE($2, title), description = COALESCE($3, description)
		WHERE id = $1::uuid`, id, title, desc)
	return err
}

func (s *Store) DeleteVideo(ctx context.Context, id string, owner int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM videos WHERE id = $1::uuid AND owner_id = $2`, id, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// VideoReady: mp4 — качество, для которого собран MP4 ("" — не собран).
func (s *Store) VideoReady(ctx context.Context, id string, seconds, w, h int, qualities []string, mp4 string) (int64, error) {
	var owner int64
	err := s.pool.QueryRow(ctx, `
		UPDATE videos SET status = 'ready', duration_sec = $2, width = $3, height = $4,
		                  renditions = $5, ready_at = now(), error = '', mp4 = NULLIF($6, '')
		WHERE id = $1::uuid AND status = 'processing' RETURNING owner_id`, id, seconds, w, h, qualities, mp4).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound // видео удалили, пока оно обрабатывалось
	}
	return owner, err
}

func (s *Store) VideoFailed(ctx context.Context, id, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE videos SET status = 'failed', error = $2 WHERE id = $1::uuid AND status = 'processing'`, id, truncate(msg, 500))
	return err
}

// ---------- очередь задач ----------

type Job struct {
	ID       int64
	Kind     string
	Payload  map[string]string
	Attempts int
}

// ClaimJob забирает следующую задачу. SKIP LOCKED: параллельные воркеры
// не ждут друг друга и не берут одну задачу дважды.
func (s *Store) ClaimJob(ctx context.Context) (*Job, error) {
	j := &Job{}
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		UPDATE jobs SET status = 'running', locked_at = now(), attempts = attempts + 1
		WHERE id = (
			SELECT id FROM jobs WHERE status = 'queued' AND run_after <= now()
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, kind, payload, attempts`).Scan(&j.ID, &j.Kind, &payload, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return j, json.Unmarshal(payload, &j.Payload)
}

func (s *Store) CompleteJob(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'done', locked_at = NULL WHERE id = $1`, id)
	return err
}

// FailJob: до 3 попыток с паузой, потом окончательно failed.
func (s *Store) FailJob(ctx context.Context, id int64, msg string, retryAfter time.Duration) (final bool, err error) {
	err = s.pool.QueryRow(ctx, `
		UPDATE jobs SET
			status = CASE WHEN attempts >= 3 THEN 'failed' ELSE 'queued' END,
			run_after = now() + $3::interval, locked_at = NULL, error = $2
		WHERE id = $1 RETURNING status = 'failed'`, id, truncate(msg, 1000), retryAfter).Scan(&final)
	return final, err
}

// RequeueStale возвращает в очередь задачи упавших воркеров.
func (s *Store) RequeueStale(ctx context.Context, after time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status = 'queued', locked_at = NULL
		WHERE status = 'running' AND locked_at < now() - $1::interval`, after)
	return tag.RowsAffected(), err
}

func (s *Store) SetVideoMP4(ctx context.Context, id, rendition string) error {
	_, err := s.pool.Exec(ctx, `UPDATE videos SET mp4 = $2 WHERE id = $1::uuid AND status = 'ready'`, id, rendition)
	return err
}

// EnqueueMP4Backfill ставит в очередь сборку MP4 для видео, обработанных до
// появления MP4. Повторный вызов не дублирует задачи.
func (s *Store) EnqueueMP4Backfill(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (kind, payload)
		SELECT 'video_mp4', jsonb_build_object('video_id', v.id::text, 'rendition', v.renditions[array_length(v.renditions, 1)])
		FROM videos v
		WHERE v.status = 'ready' AND v.mp4 IS NULL AND array_length(v.renditions, 1) > 0
		  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.kind = 'video_mp4' AND j.status IN ('queued', 'running')
		                  AND j.payload->>'video_id' = v.id::text)`)
	return tag.RowsAffected(), err
}
