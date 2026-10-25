package media

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Storage — где лежат файлы. Сейчас локальный диск; для S3/MinIO
// достаточно написать другую реализацию этого интерфейса.
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
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (d *Disk) Open(id, size string) (io.ReadSeekCloser, time.Time, error) {
	p, err := d.path(id, size)
	if err != nil {
		return nil, time.Time{}, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
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
