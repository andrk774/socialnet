package store

import (
	"context"
	"encoding/json"
)

type CommentContent struct {
	MediaIDs    []string `json:"photo_ids"`
	TrackIDs    []string `json:"track_ids"`
	VideoIDs    []string `json:"video_ids"`
	DocumentIDs []string `json:"document_ids"`
	Sticker     string   `json:"sticker"`
}

func (s *Store) CommentReact(ctx context.Context, id, user int64, reaction string) error {
	if reaction == "" {
		_, err := s.pool.Exec(ctx, `DELETE FROM comment_reactions WHERE comment_id=$1 AND user_id=$2`, id, user)
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO comment_reactions(comment_id,user_id,reaction) VALUES($1,$2,$3) ON CONFLICT(comment_id,user_id) DO UPDATE SET reaction=excluded.reaction`, id, user, reaction)
	return err
}

type CommentReactor struct {
	User     UserCard `json:"user"`
	Reaction string   `json:"reaction"`
}

func (s *Store) CommentReactors(ctx context.Context, id, after int64, limit int) ([]CommentReactor, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id,u.username,u.first_name,u.last_name,u.avatar_media_id::text,r.reaction FROM comment_reactions r JOIN users u ON u.id=r.user_id WHERE r.comment_id=$1 AND u.id>$2 AND u.banned_at IS NULL ORDER BY u.id LIMIT $3`, id, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CommentReactor{}
	for rows.Next() {
		var x CommentReactor
		var avatar *string
		if err := rows.Scan(&x.User.ID, &x.User.Username, &x.User.FirstName, &x.User.LastName, &avatar, &x.Reaction); err != nil {
			return nil, err
		}
		x.User.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, x)
	}
	return out, rows.Err()
}

// Metadata is resolved at read time so removed media does not remain in comments.
func (s *Store) commentContent(ctx context.Context, c *Comment, viewer int64) error {
	var photoIDs []string
	var tracks, videos, docs []byte
	err := s.pool.QueryRow(ctx, `SELECT cm.media_ids,cm.sticker,
 (SELECT count(*) FROM comment_reactions WHERE comment_id=cm.id),
 COALESCE((SELECT reaction FROM comment_reactions WHERE comment_id=cm.id AND user_id=$2),''),
 COALESCE((SELECT json_agg(json_build_object('id',id,'title',title,'artist',artist,'duration_sec',duration_sec)) FROM tracks WHERE id=ANY(cm.track_ids) AND removed_at IS NULL),'[]'),
 COALESCE((SELECT json_agg(json_build_object('id',id,'title',title,'duration_sec',duration_sec,'mp4_url',CASE WHEN mp4 IS NOT NULL THEN '/video/'||id||'/'||mp4||'.mp4' END)) FROM videos WHERE id=ANY(cm.video_ids) AND status='ready'),'[]'),
 COALESCE((SELECT json_agg(json_build_object('id',id,'title',title,'ext',ext,'size',size_bytes)) FROM documents WHERE id=ANY(cm.document_ids) AND deleted_at IS NULL AND hidden_at IS NULL),'[]')
 FROM comments cm WHERE cm.id=$1`, c.ID, viewer).Scan(&photoIDs, &c.Sticker, &c.LikesCount, &c.MyReaction, &tracks, &videos, &docs)
	if err != nil {
		return err
	}
	c.Photos = []Photo{}
	for _, id := range photoIDs {
		c.Photos = append(c.Photos, Photo{ID: id, URL: *MediaURL(&id, "full"), ThumbURL: *MediaURL(&id, "thumb")})
	}
	if err = json.Unmarshal(tracks, &c.Tracks); err != nil {
		return err
	}
	if err = json.Unmarshal(videos, &c.Videos); err != nil {
		return err
	}
	if err = json.Unmarshal(docs, &c.Documents); err != nil {
		return err
	}
	for i := range c.Tracks {
		c.Tracks[i].URL = AudioURL(c.Tracks[i].ID)
	}
	for i := range c.Videos {
		v := &c.Videos[i]
		v.HLSURL = VideoURL(v.ID, "master.m3u8")
		v.PosterURL = VideoURL(v.ID, "poster.jpg")
	}
	for i := range c.Documents {
		c.Documents[i].URL = DocumentURL(c.Documents[i].ID)
	}
	return nil
}
