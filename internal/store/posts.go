package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type CommunityRef struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type Photo struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	ThumbURL string `json:"thumb_url"`
}

// PostTrack и PostVideo — вложения поста.
type PostTrack struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	DurationSec int    `json:"duration_sec"`
	URL         string `json:"url"`
}

type PostVideo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	DurationSec int    `json:"duration_sec"`
	PosterURL   string `json:"poster_url"`
	HLSURL      string `json:"hls_url"`
	MP4URL      string `json:"mp4_url,omitempty"` // для браузеров без HLS
}

func VideoURL(id, file string) string { return "/video/" + id + "/" + file }

type Post struct {
	ID            int64          `json:"id"`
	Author        UserCard       `json:"author"`
	Community     *CommunityRef  `json:"community,omitempty"`
	Text          string         `json:"text"`
	Photos        []Photo        `json:"photos"`
	Tracks        []PostTrack    `json:"tracks"`
	Videos        []PostVideo    `json:"videos"`
	LikesCount    int            `json:"likes_count"`
	CommentsCount int            `json:"comments_count"`
	LikedByMe     bool           `json:"liked_by_me"`
	MyReaction    *string        `json:"my_reaction"`
	Reactions     map[string]int `json:"reactions"`
	CreatedAt     time.Time      `json:"created_at"`
	EditedAt      *time.Time     `json:"edited_at,omitempty"`
}

// postVisible — единое правило видимости поста для зрителя $1.
// Используется во всех выборках, поэтому лента, стена, хэштеги и
// прямой GET /posts/{id} не могут разойтись в проверках.
const postVisible = `
	p.deleted_at IS NULL AND p.hidden_at IS NULL AND a.banned_at IS NULL
	AND (c.id IS NULL OR c.hidden_at IS NULL)
	-- посты закрытого сообщества видят только его участники
	AND (c.id IS NULL OR c.visibility = 'open'
	     OR EXISTS (SELECT 1 FROM community_members cmv WHERE cmv.community_id = c.id AND cmv.user_id = $1))
	AND (
		p.community_id IS NOT NULL
		OR p.author_id = $1
		OR a.profile_visibility = 'everyone'
		OR (a.profile_visibility = 'friends'
			AND EXISTS (SELECT 1 FROM follows x WHERE x.follower_id = $1 AND x.followee_id = p.author_id)
			AND EXISTS (SELECT 1 FROM follows y WHERE y.follower_id = p.author_id AND y.followee_id = $1))
	)
	AND (p.community_id IS NOT NULL OR NOT EXISTS (
		SELECT 1 FROM blocks b
		WHERE (b.blocker_id = p.author_id AND b.blocked_id = $1)
		   OR (b.blocker_id = $1 AND b.blocked_id = p.author_id)))`

const postSelect = `
	SELECT p.id, a.id, a.username, a.first_name, a.last_name, a.avatar_media_id::text,
	       c.id, c.slug, c.name,
	       p.text, p.likes_count, p.comments_count, p.created_at, p.edited_at,
	       (SELECT l.reaction FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1),
	       COALESCE((SELECT json_object_agg(reaction, n) FROM
	                   (SELECT reaction, count(*) AS n FROM post_likes WHERE post_id = p.id GROUP BY reaction) r), '{}'),
	       COALESCE((SELECT array_agg(pm.media_id::text ORDER BY pm.position)
	                 FROM post_media pm WHERE pm.post_id = p.id), '{}'),
	       COALESCE((SELECT json_agg(json_build_object('id', t.id, 'title', t.title, 'artist', t.artist,
	                         'duration_sec', t.duration_sec) ORDER BY pt.position)
	                 FROM post_tracks pt JOIN tracks t ON t.id = pt.track_id
	                 WHERE pt.post_id = p.id AND t.removed_at IS NULL), '[]'),
	       COALESCE((SELECT json_agg(json_build_object('id', v.id, 'title', v.title,
	                         'duration_sec', v.duration_sec,
	                         'mp4_url', CASE WHEN v.mp4 IS NOT NULL THEN '/video/' || v.id || '/' || v.mp4 || '.mp4' END)
	                         ORDER BY pv.position)
	                 FROM post_videos pv JOIN videos v ON v.id = pv.video_id
	                 WHERE pv.post_id = p.id AND v.status = 'ready'), '[]')
	FROM posts p
	JOIN users a ON a.id = p.author_id
	LEFT JOIN communities c ON c.id = p.community_id`

func (s *Store) queryPosts(ctx context.Context, sql string, args ...any) ([]Post, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		var (
			p         Post
			avatar    *string
			cID       *int64
			cSlug     *string
			cName     *string
			mediaIDs  []string
			tracks    []byte
			videos    []byte
			reactions []byte
		)
		if err := rows.Scan(&p.ID, &p.Author.ID, &p.Author.Username, &p.Author.FirstName, &p.Author.LastName, &avatar,
			&cID, &cSlug, &cName,
			&p.Text, &p.LikesCount, &p.CommentsCount, &p.CreatedAt, &p.EditedAt, &p.MyReaction, &reactions, &mediaIDs,
			&tracks, &videos); err != nil {
			return nil, err
		}
		p.LikedByMe = p.MyReaction != nil
		if err := json.Unmarshal(reactions, &p.Reactions); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tracks, &p.Tracks); err != nil {
			return nil, err
		}
		for i := range p.Tracks {
			p.Tracks[i].URL = AudioURL(p.Tracks[i].ID)
		}
		if err := json.Unmarshal(videos, &p.Videos); err != nil {
			return nil, err
		}
		for i := range p.Videos {
			p.Videos[i].PosterURL = VideoURL(p.Videos[i].ID, "poster.jpg")
			p.Videos[i].HLSURL = VideoURL(p.Videos[i].ID, "master.m3u8")
		}
		p.Author.AvatarURL = MediaURL(avatar, "thumb")
		if cID != nil {
			p.Community = &CommunityRef{ID: *cID, Slug: *cSlug, Name: *cName}
		}
		p.Photos = make([]Photo, 0, len(mediaIDs))
		for _, id := range mediaIDs {
			id := id
			p.Photos = append(p.Photos, Photo{ID: id, URL: *MediaURL(&id, "full"), ThumbURL: *MediaURL(&id, "thumb")})
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type NewPost struct {
	AuthorID    int64
	CommunityID *int64
	Text        string
	MediaIDs    []string
	TrackIDs    []string // любые доступные треки — музыкой делятся
	VideoIDs    []string // любые готовые видео
	Hashtags    []string
}

func (s *Store) CreatePost(ctx context.Context, np NewPost) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO posts (author_id, community_id, text) VALUES ($1, $2, $3) RETURNING id`,
		np.AuthorID, np.CommunityID, np.Text).Scan(&id); err != nil {
		return 0, err
	}

	if len(np.MediaIDs) > 0 {
		// Прикрепить можно только свои фото: чужой media_id просто не найдётся.
		tag, err := tx.Exec(ctx, `
			INSERT INTO post_media (post_id, media_id, position)
			SELECT $1, m.id, t.ord::smallint
			FROM unnest($2::uuid[]) WITH ORDINALITY AS t(id, ord)
			JOIN media m ON m.id = t.id AND m.owner_id = $3`,
			id, np.MediaIDs, np.AuthorID)
		if err != nil {
			return 0, err
		}
		if int(tag.RowsAffected()) != len(np.MediaIDs) {
			return 0, ErrBadMedia
		}
	}

	if len(np.TrackIDs) > 0 {
		tag, err := tx.Exec(ctx, `
			INSERT INTO post_tracks (post_id, track_id, position)
			SELECT $1, tr.id, t.ord::smallint
			FROM unnest($2::uuid[]) WITH ORDINALITY AS t(id, ord)
			JOIN tracks tr ON tr.id = t.id AND tr.removed_at IS NULL`, id, np.TrackIDs)
		if err != nil {
			return 0, err
		}
		if int(tag.RowsAffected()) != len(np.TrackIDs) {
			return 0, ErrBadMedia
		}
	}
	if len(np.VideoIDs) > 0 {
		tag, err := tx.Exec(ctx, `
			INSERT INTO post_videos (post_id, video_id, position)
			SELECT $1, v.id, t.ord::smallint
			FROM unnest($2::uuid[]) WITH ORDINALITY AS t(id, ord)
			JOIN videos v ON v.id = t.id AND v.status = 'ready'`, id, np.VideoIDs)
		if err != nil {
			return 0, err
		}
		if int(tag.RowsAffected()) != len(np.VideoIDs) {
			return 0, ErrBadMedia
		}
	}

	if err := setHashtags(ctx, tx, id, np.Hashtags); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func setHashtags(ctx context.Context, tx pgx.Tx, postID int64, tags []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM post_hashtags WHERE post_id = $1`, postID); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO hashtags (tag) SELECT unnest($1::citext[]) ON CONFLICT DO NOTHING`, tags); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO post_hashtags (post_id, hashtag_id)
		SELECT $1, h.id FROM hashtags h WHERE h.tag = ANY($2::citext[])
		ON CONFLICT DO NOTHING`, postID, tags)
	return err
}

func (s *Store) UpdatePostText(ctx context.Context, postID int64, text string, tags []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE posts SET text = $2, edited_at = now() WHERE id = $1`, postID, text); err != nil {
		return err
	}
	if err := setHashtags(ctx, tx, postID, tags); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeletePost(ctx context.Context, postID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE posts SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, postID)
	return err
}

// PostMeta — минимум для проверки прав: кто автор и чьей стене принадлежит.
type PostMeta struct {
	AuthorID    int64
	CommunityID *int64
}

func (s *Store) PostMeta(ctx context.Context, postID int64) (PostMeta, error) {
	var m PostMeta
	err := s.pool.QueryRow(ctx, `SELECT author_id, community_id FROM posts WHERE id = $1 AND deleted_at IS NULL`,
		postID).Scan(&m.AuthorID, &m.CommunityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// PostForViewer возвращает пост, только если зритель имеет право его видеть.
// Иначе ErrNotFound: не раскрываем, что пост существует.
func (s *Store) PostForViewer(ctx context.Context, viewer, postID int64) (*Post, error) {
	posts, err := s.queryPosts(ctx, postSelect+` WHERE p.id = $2 AND `+postVisible, viewer, postID)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, ErrNotFound
	}
	return &posts[0], nil
}

// Feed: свои посты, посты тех, на кого подписан, и сообществ, где состою.
func (s *Store) Feed(ctx context.Context, viewer, before int64, limit int) ([]Post, error) {
	return s.queryPosts(ctx, postSelect+`
		WHERE p.id < $2 AND `+postVisible+`
		  AND (
			(p.community_id IS NULL AND (p.author_id = $1 OR p.author_id IN
				(SELECT followee_id FROM follows WHERE follower_id = $1)))
			OR p.community_id IN (SELECT community_id FROM community_members WHERE user_id = $1)
		  )
		ORDER BY p.id DESC LIMIT $3`, viewer, before, limit)
}

// Wall — личная стена пользователя.
func (s *Store) Wall(ctx context.Context, viewer, authorID, before int64, limit int) ([]Post, error) {
	return s.queryPosts(ctx, postSelect+`
		WHERE p.author_id = $2 AND p.community_id IS NULL AND p.id < $3 AND `+postVisible+`
		ORDER BY p.id DESC LIMIT $4`, viewer, authorID, before, limit)
}

func (s *Store) CommunityWall(ctx context.Context, viewer, communityID, before int64, limit int) ([]Post, error) {
	return s.queryPosts(ctx, postSelect+`
		WHERE p.community_id = $2 AND p.id < $3 AND `+postVisible+`
		ORDER BY p.id DESC LIMIT $4`, viewer, communityID, before, limit)
}

func (s *Store) HashtagPosts(ctx context.Context, viewer int64, tag string, before int64, limit int) ([]Post, error) {
	return s.queryPosts(ctx, postSelect+`
		JOIN post_hashtags ph ON ph.post_id = p.id
		JOIN hashtags h ON h.id = ph.hashtag_id AND h.tag = $2::citext
		WHERE p.id < $3 AND `+postVisible+`
		ORDER BY p.id DESC LIMIT $4`, viewer, tag, before, limit)
}

var Reactions = []string{"like", "love", "haha", "wow", "sad", "angry"}

// React ставит или меняет реакцию — атомарно, вместе со счётчиком.
// xmax = 0 у вставленной строки: так отличаем новую реакцию от смены старой.
func (s *Store) React(ctx context.Context, postID, userID int64, reaction string) (count int, created bool, err error) {
	err = s.pool.QueryRow(ctx, `
		WITH up AS (
			INSERT INTO post_likes (post_id, user_id, reaction) VALUES ($1, $2, $3)
			ON CONFLICT (post_id, user_id) DO UPDATE SET reaction = EXCLUDED.reaction
			RETURNING (xmax = 0) AS inserted
		)
		UPDATE posts SET likes_count = likes_count + (SELECT count(*) FROM up WHERE inserted)
		WHERE id = $1
		RETURNING likes_count, COALESCE((SELECT bool_or(inserted) FROM up), false)`,
		postID, userID, reaction).Scan(&count, &created)
	return count, created, err
}

// Like — реакция «нравится» (старый эндпоинт /like).
func (s *Store) Like(ctx context.Context, postID, userID int64) (int, bool, error) {
	return s.React(ctx, postID, userID, "like")
}

func (s *Store) Unlike(ctx context.Context, postID, userID int64) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		WITH del AS (
			DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2 RETURNING 1
		)
		UPDATE posts SET likes_count = likes_count - (SELECT count(*) FROM del)
		WHERE id = $1
		RETURNING likes_count`, postID, userID).Scan(&count)
	return count, err
}

// ---------- комментарии ----------

type Comment struct {
	ID           int64     `json:"id"`
	PostID       int64     `json:"post_id"`
	ParentID     *int64    `json:"parent_id,omitempty"` // корневой комментарий ветки
	ReplyTo      *UserCard `json:"reply_to,omitempty"`  // кому ответили
	RepliesCount int       `json:"replies_count"`
	Author       UserCard  `json:"author"`
	Text         string    `json:"text"`
	CreatedAt    time.Time `json:"created_at"`
}

// AddComment: parentID — корень ветки (nil для корневого), replyTo — на что ответили.
func (s *Store) AddComment(ctx context.Context, postID, authorID int64, text string, parentID, replyTo *int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		WITH c AS (
			INSERT INTO comments (post_id, author_id, text, parent_id, reply_to_id)
			VALUES ($1, $2, $3, $4, $5) RETURNING id
		), root AS (
			UPDATE comments SET replies_count = replies_count + 1 WHERE id = $4
		)
		UPDATE posts SET comments_count = comments_count + 1
		WHERE id = $1
		RETURNING (SELECT id FROM c)`, postID, authorID, text, parentID, replyTo).Scan(&id)
	return id, err
}

type CommentMeta struct {
	PostID   int64
	AuthorID int64
	ParentID *int64
}

func (s *Store) CommentMeta(ctx context.Context, id int64) (CommentMeta, error) {
	var m CommentMeta
	err := s.pool.QueryRow(ctx, `
		SELECT post_id, author_id, parent_id FROM comments WHERE id = $1 AND deleted_at IS NULL AND hidden_at IS NULL`,
		id).Scan(&m.PostID, &m.AuthorID, &m.ParentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// DeleteComment: удаление корня убирает и всю ветку; счётчики поправляются.
func (s *Store) DeleteComment(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		WITH d AS (
			UPDATE comments SET deleted_at = now()
			WHERE (id = $1 OR parent_id = $1) AND deleted_at IS NULL
			RETURNING post_id, parent_id
		), root AS (
			UPDATE comments SET replies_count = replies_count - (SELECT count(*) FROM d WHERE parent_id IS NOT NULL)
			WHERE id = (SELECT parent_id FROM d WHERE parent_id IS NOT NULL AND parent_id <> $1 LIMIT 1)
		)
		UPDATE posts SET comments_count = comments_count - (SELECT count(*) FROM d)
		WHERE id = (SELECT post_id FROM d LIMIT 1)`, id)
	return err
}

const commentSelect = `
	SELECT cm.id, cm.post_id, cm.parent_id, cm.replies_count,
	       u.id, u.username, u.first_name, u.last_name, u.avatar_media_id::text, cm.text, cm.created_at,
	       ru.id, ru.username, ru.first_name, ru.last_name
	FROM comments cm
	JOIN users u ON u.id = cm.author_id
	LEFT JOIN comments rc ON rc.id = cm.reply_to_id
	LEFT JOIN users ru ON ru.id = rc.author_id
	WHERE cm.deleted_at IS NULL AND cm.hidden_at IS NULL AND u.banned_at IS NULL
	  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE b.blocker_id = $1 AND b.blocked_id = cm.author_id)`

func (s *Store) queryComments(ctx context.Context, sql string, args ...any) ([]Comment, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var (
			c                    Comment
			avatar               *string
			rID                  *int64
			rUser, rFirst, rLast *string
		)
		if err := rows.Scan(&c.ID, &c.PostID, &c.ParentID, &c.RepliesCount,
			&c.Author.ID, &c.Author.Username, &c.Author.FirstName, &c.Author.LastName, &avatar, &c.Text, &c.CreatedAt,
			&rID, &rUser, &rFirst, &rLast); err != nil {
			return nil, err
		}
		c.Author.AvatarURL = MediaURL(avatar, "thumb")
		if rID != nil && c.ParentID != nil {
			c.ReplyTo = &UserCard{ID: *rID, Username: *rUser, FirstName: *rFirst, LastName: *rLast}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Comments — корневые комментарии поста (ответы — через Replies).
func (s *Store) Comments(ctx context.Context, viewer, postID, after int64, limit int) ([]Comment, error) {
	return s.queryComments(ctx, commentSelect+`
		AND cm.post_id = $2 AND cm.parent_id IS NULL AND cm.id > $3
		ORDER BY cm.id LIMIT $4`, viewer, postID, after, limit)
}

// Replies — ответы в ветке.
func (s *Store) Replies(ctx context.Context, viewer, rootID, after int64, limit int) ([]Comment, error) {
	return s.queryComments(ctx, commentSelect+`
		AND cm.parent_id = $2 AND cm.id > $3
		ORDER BY cm.id LIMIT $4`, viewer, rootID, after, limit)
}
