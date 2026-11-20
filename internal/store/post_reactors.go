package store

import "context"

type PostReactor struct {
	User     UserCard `json:"user"`
	Reaction string   `json:"reaction"`
}

func (s *Store) PostReactors(ctx context.Context, postID, after int64, limit int) ([]PostReactor, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+cardCols+`, l.reaction FROM post_likes l JOIN users u ON u.id=l.user_id WHERE l.post_id=$1 AND u.id>$2 ORDER BY u.id LIMIT $3`, postID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PostReactor{}
	for rows.Next() {
		var item PostReactor
		var avatar *string
		if err := rows.Scan(&item.User.ID, &item.User.Username, &item.User.FirstName, &item.User.LastName, &avatar, &item.Reaction); err != nil {
			return nil, err
		}
		item.User.AvatarURL = MediaURL(avatar, "thumb")
		items = append(items, item)
	}
	return items, rows.Err()
}
