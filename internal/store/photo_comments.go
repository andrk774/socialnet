package store

import (
	"context"
	"time"
)

// Photo comments use the same audience checks as posts containing the photo.
func (s *Store) CanReadPhotoComments(ctx context.Context, viewer int64, photo string) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media m WHERE m.id=$2::uuid AND (m.owner_id=$1 OR EXISTS(SELECT 1 FROM posts p JOIN users a ON a.id=p.author_id LEFT JOIN communities c ON c.id=p.community_id JOIN post_media pm ON pm.post_id=p.id WHERE pm.media_id=m.id AND `+postVisible+`)))`, viewer, photo).Scan(&allowed)
	return allowed, err
}

type PhotoComment struct {
	ID        int64     `json:"id"`
	AuthorID  int64     `json:"author_id"`
	Username  string    `json:"username"`
	Name      string    `json:"name"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) PhotoComments(ctx context.Context, photo string, after int64) ([]PhotoComment, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.id,c.author_id,u.username,concat_ws(' ',u.first_name,u.last_name),c.text,c.created_at FROM photo_comments c JOIN users u ON u.id=c.author_id WHERE c.photo_id=$1::uuid AND c.id>$2 ORDER BY c.id LIMIT 51`, photo, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PhotoComment{}
	for rows.Next() {
		var c PhotoComment
		if err := rows.Scan(&c.ID, &c.AuthorID, &c.Username, &c.Name, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
