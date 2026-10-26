package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Track struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	DurationSec int       `json:"duration_sec"`
	URL         string    `json:"url"`
	UploaderID  int64     `json:"uploader_id"`
	InMyMusic   bool      `json:"in_my_music"`
	CreatedAt   time.Time `json:"created_at"`
	Removed     bool      `json:"-"`
	seq         int64
}

func AudioURL(id string) string { return "/audio/" + id + ".mp3" }

// IsBlocked: файл с таким хэшем снят по жалобе правообладателя.
func (s *Store) IsBlocked(ctx context.Context, sha []byte) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM content_blocklist WHERE sha256 = $1)`, sha).Scan(&ok)
	return ok, err
}

type NewTrack struct {
	ID, Title, Artist string
	UploaderID        int64
	DurationSec       int
	Size              int64
	SourceSHA         []byte
}

// CreateTrack сохраняет трек и сразу добавляет его в «Мою музыку» загрузившего.
func (s *Store) CreateTrack(ctx context.Context, t NewTrack) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tracks (id, uploader_id, title, artist, duration_sec, size_bytes, source_sha256)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)`,
			t.ID, t.UploaderID, t.Title, t.Artist, t.DurationSec, t.Size, t.SourceSHA); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO user_tracks (user_id, track_id) VALUES ($1, $2::uuid)`, t.UploaderID, t.ID)
		return err
	})
}

const trackSelect = `
	SELECT t.id::text, t.title, t.artist, t.duration_sec, t.uploader_id, t.created_at, t.removed_at IS NOT NULL,
	       EXISTS (SELECT 1 FROM user_tracks m WHERE m.user_id = $1 AND m.track_id = t.id)`

func (s *Store) queryTracks(ctx context.Context, withSeq bool, sql string, args ...any) ([]Track, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Track{}
	for rows.Next() {
		var t Track
		dest := []any{&t.ID, &t.Title, &t.Artist, &t.DurationSec, &t.UploaderID, &t.CreatedAt, &t.Removed, &t.InMyMusic}
		if withSeq {
			dest = append(dest, &t.seq)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		t.URL = AudioURL(t.ID)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Track(ctx context.Context, viewer int64, id string) (*Track, error) {
	list, err := s.queryTracks(ctx, false, trackSelect+` FROM tracks t WHERE t.id = $2::uuid`, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// MyMusic — «Моя музыка», последние добавленные сверху. Курсор — seq.
func (s *Store) MyMusic(ctx context.Context, viewer, owner, before int64, limit int) ([]Track, int64, error) {
	list, err := s.queryTracks(ctx, true, trackSelect+`, ut.seq
		FROM user_tracks ut JOIN tracks t ON t.id = ut.track_id
		WHERE ut.user_id = $2 AND ut.seq < $3 AND t.removed_at IS NULL
		ORDER BY ut.seq DESC LIMIT $4`, viewer, owner, before, limit)
	var cursor int64
	if len(list) > 0 {
		cursor = list[len(list)-1].seq
	}
	return list, cursor, err
}

func (s *Store) AddToMyMusic(ctx context.Context, user int64, track string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_tracks (user_id, track_id) VALUES ($1, $2::uuid) ON CONFLICT DO NOTHING`, user, track)
	return err
}

func (s *Store) RemoveFromMyMusic(ctx context.Context, user int64, track string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_tracks WHERE user_id = $1 AND track_id = $2::uuid`, user, track)
	return err
}

func (s *Store) SearchTracks(ctx context.Context, viewer int64, q string, limit int) ([]Track, error) {
	return s.queryTracks(ctx, false, trackSelect+`
		FROM tracks t
		WHERE lower(t.artist || ' ' || t.title) LIKE $2 AND t.removed_at IS NULL
		ORDER BY similarity(lower(t.artist || ' ' || t.title), lower($3)) DESC, t.created_at DESC
		LIMIT $4`, viewer, likePattern(q), q, limit)
}

// DeleteTrack — удаление загрузившим (из всех плейлистов и постов тоже).
func (s *Store) DeleteTrack(ctx context.Context, id string, uploader int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM tracks WHERE id = $1::uuid AND uploader_id = $2`, id, uploader)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Takedown снимает трек или видео по жалобе и запоминает хэш исходника,
// чтобы тот же файл нельзя было загрузить снова. hidden=false — восстановить.
func (s *Store) Takedown(ctx context.Context, kind, id string, hidden bool, reason string) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var sha []byte
		var err error
		switch kind {
		case "track":
			err = tx.QueryRow(ctx, `
				UPDATE tracks SET removed_at = CASE WHEN $2 THEN COALESCE(removed_at, now()) ELSE NULL END
				WHERE id = $1::uuid RETURNING source_sha256`, id, hidden).Scan(&sha)
		case "video":
			err = tx.QueryRow(ctx, `
				UPDATE videos SET status = CASE WHEN $2 THEN 'removed'
					WHEN ready_at IS NOT NULL THEN 'ready' ELSE 'failed' END
				WHERE id = $1::uuid RETURNING source_sha256`, id, hidden).Scan(&sha)
		case "document":
			err = tx.QueryRow(ctx, `
				UPDATE documents SET hidden_at = CASE WHEN $2 THEN COALESCE(hidden_at, now()) ELSE NULL END
				WHERE id = $1::uuid RETURNING sha256`, id, hidden).Scan(&sha)
		default:
			return ErrNotFound
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if hidden {
			_, err = tx.Exec(ctx, `
				INSERT INTO content_blocklist (sha256, kind, reason) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				sha, kind, reason)
		} else {
			_, err = tx.Exec(ctx, `DELETE FROM content_blocklist WHERE sha256 = $1`, sha)
		}
		return err
	})
}

// ---------- плейлисты ----------

type Playlist struct {
	ID         int64     `json:"id"`
	OwnerID    int64     `json:"owner_id"`
	Title      string    `json:"title"`
	TrackCount int       `json:"track_count"`
	Tracks     []Track   `json:"tracks,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) CreatePlaylist(ctx context.Context, owner int64, title string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO playlists (owner_id, title) VALUES ($1, $2) RETURNING id`, owner, title).Scan(&id)
	return id, err
}

func (s *Store) Playlist(ctx context.Context, viewer, id int64) (*Playlist, error) {
	p := &Playlist{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, owner_id, title, created_at,
		       (SELECT count(*) FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id
		        WHERE pt.playlist_id = p.id AND t.removed_at IS NULL)
		FROM playlists p WHERE id = $1`, id).Scan(&p.ID, &p.OwnerID, &p.Title, &p.CreatedAt, &p.TrackCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Tracks, err = s.queryTracks(ctx, false, trackSelect+`
		FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id
		WHERE pt.playlist_id = $2 AND t.removed_at IS NULL
		ORDER BY pt.position`, viewer, id)
	return p, err
}

func (s *Store) Playlists(ctx context.Context, owner int64) ([]Playlist, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, owner_id, title, created_at,
		       (SELECT count(*) FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id
		        WHERE pt.playlist_id = p.id AND t.removed_at IS NULL)
		FROM playlists p WHERE owner_id = $1 ORDER BY id DESC LIMIT 200`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Playlist{}
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Title, &p.CreatedAt, &p.TrackCount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) RenamePlaylist(ctx context.Context, id int64, title string) error {
	_, err := s.pool.Exec(ctx, `UPDATE playlists SET title = $2 WHERE id = $1`, id, title)
	return err
}

func (s *Store) DeletePlaylist(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM playlists WHERE id = $1`, id)
	return err
}

// AddPlaylistTrack добавляет в конец. Не больше 1000 треков в плейлисте.
func (s *Store) AddPlaylistTrack(ctx context.Context, playlist int64, track string) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO playlist_tracks (playlist_id, track_id, position)
		SELECT $1, t.id, COALESCE((SELECT max(position) FROM playlist_tracks WHERE playlist_id = $1), 0) + 1
		FROM tracks t
		WHERE t.id = $2::uuid AND t.removed_at IS NULL
		  AND (SELECT count(*) FROM playlist_tracks WHERE playlist_id = $1) < 1000
		ON CONFLICT DO NOTHING`, playlist, track)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2::uuid)`,
			playlist, track).Scan(&exists)
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) RemovePlaylistTrack(ctx context.Context, playlist int64, track string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2::uuid`, playlist, track)
	return err
}
