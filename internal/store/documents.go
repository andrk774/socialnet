package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Document struct {
	ID        string    `json:"id"`
	OwnerID   int64     `json:"owner_id"`
	Title     string    `json:"title"`
	Ext       string    `json:"ext"`
	Size      int64     `json:"size"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	seq       int64
}

func (d Document) Cursor() int64 { return d.seq }

const docCols = `id::text, owner_id, title, ext, size_bytes, created_at, seq`

func scanDocs(rows pgx.Rows) ([]Document, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Document, error) {
		var d Document
		err := r.Scan(&d.ID, &d.OwnerID, &d.Title, &d.Ext, &d.Size, &d.CreatedAt, &d.seq)
		d.URL = DocumentURL(d.ID)
		return d, err
	})
}

type NewDocument struct {
	ID     string
	Owner  int64
	Title  string
	Ext    string
	Size   int64
	SHA256 []byte
}

func (s *Store) CreateDocument(ctx context.Context, d NewDocument) (*Document, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO documents (id, owner_id, title, ext, size_bytes, sha256) VALUES ($1::uuid, $2, $3, $4, $5, $6)
		RETURNING `+docCols, d.ID, d.Owner, d.Title, d.Ext, d.Size, d.SHA256)
	if err != nil {
		return nil, err
	}
	list, err := scanDocs(rows)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// Document — живой (не удалён, не снят модератором) документ.
func (s *Store) Document(ctx context.Context, id string) (*Document, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+docCols+` FROM documents
		WHERE id = $1::uuid AND deleted_at IS NULL AND hidden_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanDocs(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

func (s *Store) MyDocuments(ctx context.Context, owner, before int64, limit int) ([]Document, error) {
	if before <= 0 {
		before = 1<<63 - 1
	}
	rows, err := s.pool.Query(ctx, `SELECT `+docCols+` FROM documents
		WHERE owner_id = $1 AND deleted_at IS NULL AND hidden_at IS NULL AND seq < $2
		ORDER BY seq DESC LIMIT $3`, owner, before, limit)
	if err != nil {
		return nil, err
	}
	return scanDocs(rows)
}

func (s *Store) RenameDocument(ctx context.Context, id string, owner int64, title string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE documents SET title = $3 WHERE id = $1::uuid AND owner_id = $2 AND deleted_at IS NULL`,
		id, owner, title)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteDocument помечает документ удалённым: он пропадёт из постов и
// сообщений, а файл удалит вызывающий код.
func (s *Store) DeleteDocument(ctx context.Context, id string, owner int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE documents SET deleted_at = now() WHERE id = $1::uuid AND owner_id = $2 AND deleted_at IS NULL`,
		id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DocumentAccess: скачать документ может владелец, тот, кому виден пост с
// этим документом, и участники бесед, где документ отправлен.
func (s *Store) DocumentAccess(ctx context.Context, viewer int64, id string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM documents WHERE id = $2::uuid AND owner_id = $1)
		    OR EXISTS (SELECT 1 FROM post_documents pd
		               JOIN posts p ON p.id = pd.post_id
		               JOIN users a ON a.id = p.author_id
		               LEFT JOIN communities c ON c.id = p.community_id
		               WHERE pd.document_id = $2::uuid AND `+postVisible+`)
		    OR EXISTS (SELECT 1 FROM message_attachments ma
		               JOIN messages m ON m.id = ma.message_id
		               JOIN conversation_state st ON st.conversation_id = m.conversation_id AND st.user_id = $1
		               WHERE ma.document_id = $2::uuid)`, viewer, id).Scan(&ok)
	return ok, err
}
