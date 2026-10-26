package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Story struct {
	ID         int64      `json:"id"`
	AuthorID   int64      `json:"author_id"`
	Photo      *Photo     `json:"photo,omitempty"`
	Video      *PostVideo `json:"video,omitempty"`
	Caption    string     `json:"caption"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	Seen       bool       `json:"seen"`
	ViewsCount *int       `json:"views_count,omitempty"` // только автору
}

type StoryGroup struct {
	Author  UserCard `json:"author"`
	AllSeen bool     `json:"all_seen"`
	Stories []Story  `json:"stories"`
}

// authorVisible — видимость автора историй для зрителя $1 (как профиль).
const authorVisible = `
	a.banned_at IS NULL AND (
		a.id = $1 OR a.profile_visibility = 'everyone'
		OR (a.profile_visibility = 'friends'
			AND EXISTS (SELECT 1 FROM follows x WHERE x.follower_id = $1 AND x.followee_id = a.id)
			AND EXISTS (SELECT 1 FROM follows y WHERE y.follower_id = a.id AND y.followee_id = $1)))
	AND NOT EXISTS (SELECT 1 FROM blocks b
		WHERE (b.blocker_id = a.id AND b.blocked_id = $1) OR (b.blocker_id = $1 AND b.blocked_id = a.id))`

// CreateStory: фото — своё, видео — своё и обработанное.
func (s *Store) CreateStory(ctx context.Context, author int64, mediaID, videoID *string, caption string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO stories (author_id, media_id, video_id, caption)
		SELECT $1, $2::uuid, $3::uuid, $4
		WHERE ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM media WHERE id = $2::uuid AND owner_id = $1))
		  AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM videos WHERE id = $3::uuid AND owner_id = $1 AND status = 'ready'))
		RETURNING id`, author, mediaID, videoID, caption).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrBadMedia
	}
	return id, err
}

const storySelect = `
	SELECT st.id, st.author_id, st.media_id::text, st.video_id::text, v.title, COALESCE(v.duration_sec, 0), v.mp4,
	       st.caption, st.created_at, st.expires_at,
	       EXISTS (SELECT 1 FROM story_views sv WHERE sv.story_id = st.id AND sv.viewer_id = $1),
	       CASE WHEN st.author_id = $1 THEN (SELECT count(*) FROM story_views sv WHERE sv.story_id = st.id) END,
	       a.id, a.username, a.first_name, a.last_name, a.avatar_media_id::text
	FROM stories st
	JOIN users a ON a.id = st.author_id
	LEFT JOIN videos v ON v.id = st.video_id
	WHERE st.expires_at > now() AND (st.video_id IS NULL OR v.status = 'ready') AND ` + authorVisible

func (s *Store) queryStories(ctx context.Context, sql string, args ...any) ([]StoryGroup, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []StoryGroup
	idx := map[int64]int{}
	for rows.Next() {
		var (
			st           Story
			mediaID, vid *string
			vTitle       *string
			vDur         int
			vMP4         *string
			views        *int
			author       UserCard
			avatar       *string
		)
		if err := rows.Scan(&st.ID, &st.AuthorID, &mediaID, &vid, &vTitle, &vDur, &vMP4, &st.Caption, &st.CreatedAt,
			&st.ExpiresAt, &st.Seen, &views, &author.ID, &author.Username, &author.FirstName, &author.LastName, &avatar); err != nil {
			return nil, err
		}
		st.ViewsCount = views
		if mediaID != nil {
			st.Photo = &Photo{ID: *mediaID, URL: *MediaURL(mediaID, "full"), ThumbURL: *MediaURL(mediaID, "thumb")}
		}
		if vid != nil {
			st.Video = &PostVideo{ID: *vid, DurationSec: vDur, PosterURL: VideoURL(*vid, "poster.jpg"), HLSURL: VideoURL(*vid, "master.m3u8")}
			if vTitle != nil {
				st.Video.Title = *vTitle
			}
			if vMP4 != nil {
				st.Video.MP4URL = VideoURL(*vid, *vMP4+".mp4")
			}
		}
		i, ok := idx[author.ID]
		if !ok {
			author.AvatarURL = MediaURL(avatar, "thumb")
			groups = append(groups, StoryGroup{Author: author, AllSeen: true})
			i = len(groups) - 1
			idx[author.ID] = i
		}
		groups[i].Stories = append(groups[i].Stories, st)
		groups[i].AllSeen = groups[i].AllSeen && st.Seen
	}
	if groups == nil {
		groups = []StoryGroup{}
	}
	return groups, rows.Err()
}

// StoriesFeed: свои и тех, на кого подписан. Сначала с непросмотренными.
func (s *Store) StoriesFeed(ctx context.Context, viewer int64) ([]StoryGroup, error) {
	groups, err := s.queryStories(ctx, storySelect+`
		AND (st.author_id = $1 OR st.author_id IN (SELECT followee_id FROM follows WHERE follower_id = $1))
		ORDER BY st.author_id = $1 DESC, st.author_id, st.id`, viewer)
	if err != nil {
		return nil, err
	}
	// свои — первыми, дальше непросмотренные, затем просмотренные
	out := make([]StoryGroup, 0, len(groups))
	for _, pass := range []func(StoryGroup) bool{
		func(g StoryGroup) bool { return g.Author.ID == viewer },
		func(g StoryGroup) bool { return g.Author.ID != viewer && !g.AllSeen },
		func(g StoryGroup) bool { return g.Author.ID != viewer && g.AllSeen },
	} {
		for _, g := range groups {
			if pass(g) {
				out = append(out, g)
			}
		}
	}
	return out, nil
}

func (s *Store) UserStories(ctx context.Context, viewer, author int64) ([]StoryGroup, error) {
	return s.queryStories(ctx, storySelect+` AND st.author_id = $2 ORDER BY st.id`, viewer, author)
}

// ViewStory отмечает просмотр, если зрителю история видна (и это не автор).
func (s *Store) ViewStory(ctx context.Context, viewer, storyID int64) (bool, error) {
	var visible bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM stories st JOIN users a ON a.id = st.author_id
		               WHERE st.id = $2 AND st.expires_at > now() AND `+authorVisible+`)`, viewer, storyID).Scan(&visible)
	if err != nil || !visible {
		return false, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO story_views (story_id, viewer_id)
		SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM stories WHERE id = $1 AND author_id = $2)
		ON CONFLICT DO NOTHING`, storyID, viewer)
	return true, err
}

// StoryViewers — только автору (проверка в условии author_id).
func (s *Store) StoryViewers(ctx context.Context, author, storyID int64) ([]UserCard, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM stories WHERE id = $1 AND author_id = $2)`,
		storyID, author).Scan(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	return s.scanCards(ctx, `
		SELECT `+cardCols+` FROM story_views sv JOIN users u ON u.id = sv.viewer_id
		WHERE sv.story_id = $1 ORDER BY sv.viewed_at DESC LIMIT 500`, storyID)
}

func (s *Store) DeleteStory(ctx context.Context, author, storyID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM stories WHERE id = $1 AND author_id = $2`, storyID, author)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
