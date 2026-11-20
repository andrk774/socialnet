package store

import (
	"context"
	"time"
)

func (s *Store) PopularCommunities(ctx context.Context, viewer int64) ([]Community, error) {
	rows, err := s.pool.Query(ctx, communitySelect+` WHERE c.hidden_at IS NULL AND c.visibility='open' ORDER BY (SELECT count(*) FROM community_members m WHERE m.community_id=c.id) DESC,c.id DESC LIMIT 12`, viewer)
	if err != nil {
		return nil, err
	}
	return scanCommunities(rows)
}

type LibraryPhoto struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	ThumbURL    string    `json:"thumb_url"`
	AlbumID     *int64    `json:"album_id"`
	Archived    bool      `json:"archived"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
type PhotoAlbum struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Count    int     `json:"count"`
	CoverURL *string `json:"cover_url"`
}

func (s *Store) LibraryPhotos(ctx context.Context, owner int64, after string, limit int) ([]LibraryPhoto, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, album_id, archived, created_at,description FROM media WHERE owner_id=$1 AND ($2='' OR id::text>$2) ORDER BY id::text LIMIT $3`, owner, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibraryPhoto{}
	for rows.Next() {
		var p LibraryPhoto
		if err := rows.Scan(&p.ID, &p.AlbumID, &p.Archived, &p.CreatedAt, &p.Description); err != nil {
			return nil, err
		}
		p.URL = *MediaURL(&p.ID, "full")
		p.ThumbURL = *MediaURL(&p.ID, "thumb")
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) DescribePhoto(ctx context.Context, owner int64, id, description string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE media SET description=$3 WHERE id=$1::uuid AND owner_id=$2`, id, owner, description)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) PhotoAlbums(ctx context.Context, owner int64) ([]PhotoAlbum, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id,a.title,(SELECT count(*) FROM media m WHERE m.album_id=a.id AND NOT m.archived),(SELECT m.id::text FROM media m WHERE m.album_id=a.id AND NOT m.archived ORDER BY m.created_at DESC LIMIT 1) FROM photo_albums a WHERE owner_id=$1 ORDER BY a.id DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PhotoAlbum{}
	for rows.Next() {
		var a PhotoAlbum
		var cover *string
		if err := rows.Scan(&a.ID, &a.Title, &a.Count, &cover); err != nil {
			return nil, err
		}
		a.CoverURL = MediaURL(cover, "thumb")
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) CreatePhotoAlbum(ctx context.Context, owner int64, title string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO photo_albums(owner_id,title) VALUES($1,$2) RETURNING id`, owner, title).Scan(&id)
	return id, err
}

// A single guarded update prevents attaching somebody else's photo or album.
func (s *Store) OrganizePhotos(ctx context.Context, owner int64, ids []string, album *int64, archived *bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if album != nil && *album != 0 {
		var ok bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM photo_albums WHERE id=$1 AND owner_id=$2)`, *album, owner).Scan(&ok)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE media SET album_id=CASE WHEN $3::bigint IS NULL THEN album_id ELSE NULLIF($3,0) END,archived=COALESCE($4,archived) WHERE owner_id=$1 AND id=ANY($2::uuid[])`, owner, ids, album, archived)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(ids) {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}
