package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"socialnet/internal/s3"
)

// Storage — где лежат файлы: локальный диск (Disk) или S3/MinIO (S3).
type Storage interface {
	Save(id, size string, data []byte) error
	Open(id, size string) (io.ReadSeekCloser, time.Time, error)
	Delete(id string) error
}

var (
	ErrNotFound = errors.New("file not found")
	idRe        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Disk struct {
	root string
}

func NewDisk(root string) (*Disk, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &Disk{root: root}, nil
}

// path строит путь только из проверенного UUID и фиксированного размера:
// никакого пользовательского ввода в пути, а значит и path traversal.
func (d *Disk) path(id, size string) (string, error) {
	if !idRe.MatchString(id) || (size != "full" && size != "thumb") {
		return "", ErrNotFound
	}
	return filepath.Join(d.root, id[:2], id+"_"+size), nil
}

func (d *Disk) Save(id, size string, data []byte) error {
	p, err := d.path(id, size)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	// пишем во временный файл и переименовываем: читатель не увидит половину файла
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (d *Disk) Open(id, size string) (io.ReadSeekCloser, time.Time, error) {
	p, err := d.path(id, size)
	if err != nil {
		return nil, time.Time{}, err
	}
	f, err := os.Open(p) //nolint:gosec // путь собран из проверенного UUID (см. path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, time.Time{}, err
	}
	return f, st.ModTime(), nil
}

func (d *Disk) Delete(id string) error {
	for _, size := range []string{"full", "thumb"} {
		p, err := d.path(id, size)
		if err != nil {
			return err
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// S3 — фото в объектном хранилище: photos/<id>/<size>.
type S3 struct {
	C *s3.Client
}

func s3Key(id, size string) (string, error) {
	if !idRe.MatchString(id) || (size != "full" && size != "thumb") {
		return "", ErrNotFound
	}
	return "photos/" + id + "/" + size, nil
}

func (s *S3) Save(id, size string, data []byte) error {
	key, err := s3Key(id, size)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return s.C.Put(ctx, key, data, http.DetectContentType(data))
}

type readSeekNopCloser struct{ *bytes.Reader }

func (readSeekNopCloser) Close() error { return nil }

// Open читает фото целиком (до 32 МБ): http.ServeContent нужен Seek.
func (s *S3) Open(id, size string) (io.ReadSeekCloser, time.Time, error) {
	key, err := s3Key(id, size)
	if err != nil {
		return nil, time.Time{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	obj, err := s.C.Get(ctx, key, "")
	if errors.Is(err, s3.ErrNotFound) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	defer func() { _ = obj.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(obj.Body, 32<<20))
	if err != nil {
		return nil, time.Time{}, err
	}
	return readSeekNopCloser{bytes.NewReader(data)}, obj.LastModified, nil
}

func (s *S3) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, size := range []string{"full", "thumb"} {
		key, err := s3Key(id, size)
		if err != nil {
			return err
		}
		if err := s.C.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
