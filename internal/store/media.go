package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateMedia(ctx context.Context, owner int64, mime string, w, h int, size int64) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO media (owner_id, mime, width, height, size_bytes) VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text`, owner, mime, w, h, size).Scan(&id)
	return id, err
}

type MediaInfo struct {
	OwnerID int64
	Mime    string
}

func (s *Store) Media(ctx context.Context, id string) (MediaInfo, error) {
	var m MediaInfo
	err := s.pool.QueryRow(ctx, `SELECT owner_id, mime FROM media WHERE id = $1::uuid`, id).Scan(&m.OwnerID, &m.Mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// DeleteMedia удаляет только своё фото. Из постов оно пропадёт (ON DELETE CASCADE),
// аватар сбросится (ON DELETE SET NULL).
func (s *Store) DeleteMedia(ctx context.Context, id string, owner int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM media WHERE id = $1::uuid AND owner_id = $2`, id, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
