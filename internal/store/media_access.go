package store

import (
	"context"
	"socialnet/internal/markup"
)

// A UUID is an identifier, not permission. Reuse the owning resource's audience.
func (s *Store) MediaAccess(ctx context.Context, viewer int64, id string) (bool, error) {
	if viewer <= 0 {
		return false, nil
	}
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM media m WHERE m.id=$2::uuid AND (
 m.owner_id=$1
 OR EXISTS (SELECT 1 FROM users a WHERE a.avatar_media_id=m.id AND `+authorVisible+`)
 OR EXISTS (SELECT 1 FROM profile_covers pc JOIN users a ON a.id=pc.user_id WHERE pc.media_id=m.id AND `+authorVisible+`)
 OR EXISTS (SELECT 1 FROM post_media pm JOIN posts p ON p.id=pm.post_id JOIN users a ON a.id=p.author_id
 LEFT JOIN communities c ON c.id=p.community_id WHERE pm.media_id=m.id AND `+postVisible+`)
 OR EXISTS (SELECT 1 FROM comments cm JOIN posts p ON p.id=cm.post_id JOIN users a ON a.id=p.author_id
 LEFT JOIN communities c ON c.id=p.community_id WHERE m.id=ANY(cm.media_ids) AND cm.deleted_at IS NULL AND cm.hidden_at IS NULL
 AND (cm.parent_id IS NULL OR EXISTS (SELECT 1 FROM comments root WHERE root.id=cm.parent_id AND root.deleted_at IS NULL AND root.hidden_at IS NULL)) AND `+postVisible+`)
 OR EXISTS (SELECT 1 FROM message_attachments ma JOIN messages msg ON msg.id=ma.message_id
 JOIN conversation_state cs ON cs.conversation_id=msg.conversation_id AND cs.user_id=$1 WHERE ma.media_id=m.id)
 OR EXISTS (SELECT 1 FROM stories st JOIN users a ON a.id=st.author_id WHERE st.media_id=m.id AND st.expires_at>now() AND `+authorVisible+`)
 OR EXISTS (SELECT 1 FROM articles a JOIN users u ON u.id=a.author_id WHERE a.cover_media_id=m.id AND `+articleVisible+`)
 OR EXISTS (SELECT 1 FROM community_emoji ce JOIN communities c ON c.id=ce.community_id WHERE ce.media_id=m.id
 AND c.hidden_at IS NULL AND (c.visibility='open' OR EXISTS(SELECT 1 FROM community_members cm WHERE cm.community_id=c.id AND cm.user_id=$1)))
 ))`, viewer, id).Scan(&allowed)
	if err != nil || allowed {
		return allowed, err
	}
	// Inline article images are parsed using the same markup grammar as rendering.
	articles, err := s.pool.Query(ctx, `SELECT a.body FROM articles a JOIN users u ON u.id=a.author_id JOIN media m ON m.owner_id=a.author_id WHERE m.id=$2::uuid AND strpos(lower(a.body),$2::text)>0 AND `+articleVisible, viewer, id)
	if err != nil {
		return false, err
	}
	for articles.Next() {
		var body string
		if err := articles.Scan(&body); err != nil {
			articles.Close()
			return false, err
		}
		blocks, err := markup.Parse(body)
		if err == nil {
			for _, mediaID := range markup.MediaIDs(blocks) {
				if mediaID == id {
					articles.Close()
					return true, nil
				}
			}
		}
	}
	err = articles.Err()
	articles.Close()
	if err != nil {
		return false, err
	}
	rows, err := s.pool.Query(ctx, `SELECT rp.route_id::text FROM route_points rp JOIN routes r ON r.id=rp.route_id WHERE rp.photo_id=$1::uuid AND rp.hidden_at IS NULL AND r.deleted_at IS NULL`, id)
	if err != nil {
		return false, err
	}
	var routes []string
	for rows.Next() {
		var route string
		if err := rows.Scan(&route); err != nil {
			rows.Close()
			return false, err
		}
		routes = append(routes, route)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, route := range routes {
		ok, err := s.RouteAccess(ctx, viewer, route)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}
