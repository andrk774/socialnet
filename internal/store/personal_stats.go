package store

import (
	"context"
	"time"
)

type PersonalStat struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	Archived  bool      `json:"archived"`
	Likes     int64     `json:"likes"`
	Comments  int64     `json:"comments"`
	Viewers   int64     `json:"viewers"`
}

type OwnComment struct {
	ID        int64     `json:"id"`
	PostID    int64     `json:"post_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) OwnComments(ctx context.Context, owner, after int64) ([]OwnComment, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,post_id,text,created_at FROM comments WHERE author_id=$1 AND id>$2 AND deleted_at IS NULL ORDER BY id LIMIT 200`, owner, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []OwnComment{}
	for rows.Next() {
		var c OwnComment
		if err := rows.Scan(&c.ID, &c.PostID, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// PersonalStats reports retained interactions during [start,end), including on older content.
func (s *Store) PersonalStats(ctx context.Context, owner int64, start, end time.Time) ([]PersonalStat, error) {
	rows, err := s.pool.Query(ctx, `
 SELECT p.id, 'post', left(p.text,200), p.created_at, false,
 (SELECT count(*) FROM post_likes l WHERE l.post_id=p.id AND l.created_at >= $2 AND l.created_at < $3),
 (SELECT count(*) FROM comments c WHERE c.post_id=p.id AND c.deleted_at IS NULL AND c.hidden_at IS NULL AND c.created_at >= $2 AND c.created_at < $3), 0::bigint
 FROM posts p WHERE p.author_id=$1 AND p.community_id IS NULL AND p.deleted_at IS NULL AND p.hidden_at IS NULL AND p.created_at < $3
 UNION ALL
 SELECT st.id, 'story', st.caption, st.created_at, st.expires_at <= now(), 0::bigint, 0::bigint,
 (SELECT count(*) FROM story_views v WHERE v.story_id=st.id AND v.viewer_id<>$1 AND v.viewed_at >= $2 AND v.viewed_at < $3)
 FROM stories st WHERE st.author_id=$1 AND st.created_at < $3
 ORDER BY 4 DESC, 1 DESC LIMIT 1001`, owner, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PersonalStat{}
	for rows.Next() {
		var item PersonalStat
		if err := rows.Scan(&item.ID, &item.Kind, &item.Text, &item.CreatedAt, &item.Archived, &item.Likes, &item.Comments, &item.Viewers); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
