package store

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Закладки: посты, видео и треки. Сама закладка ничего не раскрывает —
// при выдаче списка каждый объект заново проверяется на видимость, и то,
// что стало недоступно (пост скрыт, автор закрыл профиль), просто не показывается.

func (s *Store) AddBookmark(ctx context.Context, user int64, kind, target string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bookmarks (user_id, kind, target) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, user, kind, target)
	return err
}

func (s *Store) RemoveBookmark(ctx context.Context, user int64, kind, target string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM bookmarks WHERE user_id = $1 AND kind = $2 AND target = $3`, user, kind, target)
	return err
}

func (s *Store) CountBookmarks(ctx context.Context, user int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM bookmarks WHERE user_id = $1`, user).Scan(&n)
	return n, err
}

type Bookmark struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	Post      *Post     `json:"post,omitempty"`
	Video     *Video    `json:"video,omitempty"`
	Track     *Track    `json:"track,omitempty"`
}

type bookmarkRow struct {
	id      int64
	target  string
	created time.Time
}

// Bookmarks — страница закладок одного вида. Курсор — id закладки; next
// возвращается по числу прочитанных закладок, даже если часть отфильтрована.
func (s *Store) Bookmarks(ctx context.Context, user int64, kind string, before int64, limit int) ([]Bookmark, *int64, error) {
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, target, created_at FROM bookmarks
		WHERE user_id = $1 AND kind = $2 AND id < $3 ORDER BY id DESC LIMIT $4`, user, kind, before, limit)
	if err != nil {
		return nil, nil, err
	}
	marks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (bookmarkRow, error) {
		var b bookmarkRow
		return b, r.Scan(&b.id, &b.target, &b.created)
	})
	if err != nil {
		return nil, nil, err
	}
	var next *int64
	if len(marks) == limit {
		c := marks[len(marks)-1].id
		next = &c
	}
	// id приводятся к типу столбца, чтобы поиск шёл по первичному ключу,
	// а не перебором всей таблицы (сравнение id::text индекс не использует)
	targets := make([]string, len(marks))
	var postIDs []int64
	for i, m := range marks {
		targets[i] = m.target
		if n, err := strconv.ParseInt(m.target, 10, 64); err == nil {
			postIDs = append(postIDs, n)
		}
	}

	out := []Bookmark{}
	switch kind {
	case "post":
		posts, err := s.queryPosts(ctx, postSelect+` WHERE p.id = ANY($2::bigint[]) AND `+postVisible, user, postIDs)
		if err != nil {
			return nil, nil, err
		}
		byID := map[string]*Post{}
		for i := range posts {
			byID[strconv.FormatInt(posts[i].ID, 10)] = &posts[i]
		}
		for _, m := range marks {
			if p := byID[m.target]; p != nil {
				out = append(out, Bookmark{ID: m.id, Kind: kind, CreatedAt: m.created, Post: p})
			}
		}
	case "video":
		rows, err := s.pool.Query(ctx, `SELECT `+videoCols+` FROM videos WHERE id = ANY($1::uuid[]) AND status = 'ready'`, targets)
		if err != nil {
			return nil, nil, err
		}
		videos, err := scanVideos(rows)
		if err != nil {
			return nil, nil, err
		}
		byID := map[string]*Video{}
		for i := range videos {
			byID[videos[i].ID] = &videos[i]
		}
		for _, m := range marks {
			if v := byID[m.target]; v != nil {
				out = append(out, Bookmark{ID: m.id, Kind: kind, CreatedAt: m.created, Video: v})
			}
		}
	case "track":
		tracks, err := s.queryTracks(ctx, false, trackSelect+` FROM tracks t WHERE t.id = ANY($2::uuid[]) AND t.removed_at IS NULL`, user, targets)
		if err != nil {
			return nil, nil, err
		}
		byID := map[string]*Track{}
		for i := range tracks {
			byID[tracks[i].ID] = &tracks[i]
		}
		for _, m := range marks {
			if t := byID[m.target]; t != nil {
				out = append(out, Bookmark{ID: m.id, Kind: kind, CreatedAt: m.created, Track: t})
			}
		}
	}
	return out, next, nil
}
