package backup

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"socialnet/internal/s3"
)

// Копия медиафайлов (фото, документы, треки, видео) во внешнее хранилище —
// на случай, если пропадёт сервер вместе с диском. Файлы шифруются тем же
// ключом, что и дампы базы: у хранилища нет доступа к личным фото.
//
// Синхронизация односторонняя: новые файлы загружаются, удалённые на
// сервере (в том числе при удалении аккаунта) удаляются и из копии.
// Файлы на сервере не меняются после записи, поэтому сравнивается только
// наличие.

const (
	mediaSuffix     = ".enc"
	maxMediaBackup  = 512 << 20 // больше — пропускаем с предупреждением
	skipMediaPrefix = "tile-cache"
)

type MediaSync struct {
	Dir    string
	S3     *s3.Client
	Prefix string // "media/"
	Key    []byte
	Log    *slog.Logger
}

type MediaResult struct {
	Uploaded, Deleted, Skipped, Total int
}

// mediaFiles — относительные пути файлов, которые нужно хранить в копии.
func (m MediaSync) mediaFiles() (map[string]int64, error) {
	files := map[string]int64{}
	err := filepath.WalkDir(m.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(m.Dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == skipMediaPrefix { // кеш карты восстановится сам
				return filepath.SkipDir
			}
			return nil
		}
		// временные файлы незаконченных загрузок начинаются с точки
		if strings.HasPrefix(d.Name(), ".") || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[rel] = info.Size()
		return nil
	})
	return files, err
}

func (m MediaSync) Run(ctx context.Context) (MediaResult, error) {
	var res MediaResult
	if m.Key == nil {
		return res, errors.New("media backup needs BACKUP_ENC_KEY: files must not leave the server unencrypted")
	}
	local, err := m.mediaFiles()
	if err != nil {
		return res, err
	}
	keys, err := m.S3.List(ctx, m.Prefix)
	if err != nil {
		return res, err
	}
	remote := map[string]bool{}
	for _, k := range keys {
		remote[strings.TrimSuffix(strings.TrimPrefix(k, m.Prefix), mediaSuffix)] = true
	}
	res.Total = len(local)
	for rel, size := range local {
		if remote[rel] {
			continue
		}
		if size > maxMediaBackup {
			res.Skipped++
			m.Log.Warn("media backup: file too large, skipped", "file", rel, "size", size)
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.Dir, filepath.FromSlash(rel))) // #nosec G304 -- путь из обхода своей папки медиа
		if errors.Is(err, fs.ErrNotExist) {
			continue // удалили, пока шла синхронизация
		}
		if err != nil {
			return res, err
		}
		var enc bytes.Buffer
		if err := Encrypt(&enc, bytes.NewReader(data), m.Key); err != nil {
			return res, err
		}
		if err := m.S3.Put(ctx, m.Prefix+rel+mediaSuffix, enc.Bytes(), "application/octet-stream"); err != nil {
			return res, err
		}
		res.Uploaded++
	}
	for rel := range remote {
		if _, ok := local[rel]; ok {
			continue
		}
		if err := m.S3.Delete(ctx, m.Prefix+rel+mediaSuffix); err != nil {
			return res, err
		}
		res.Deleted++
	}
	return res, nil
}

// RestoreMedia скачивает и расшифровывает копию в Dir; существующие файлы
// не перезаписываются.
func (m MediaSync) Restore(ctx context.Context) (int, error) {
	if m.Key == nil {
		return 0, errors.New("set BACKUP_ENC_KEY")
	}
	keys, err := m.S3.List(ctx, m.Prefix)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, k := range keys {
		rel := strings.TrimSuffix(strings.TrimPrefix(k, m.Prefix), mediaSuffix)
		// ключ из чужого хранилища не должен вывести за пределы папки
		clean := filepath.Clean(filepath.FromSlash(rel))
		if rel == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return n, errors.New("media backup: suspicious key " + k)
		}
		dst := filepath.Join(m.Dir, clean)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		obj, err := m.S3.Get(ctx, k, "")
		if err != nil {
			return n, err
		}
		var plain bytes.Buffer
		err = Decrypt(&plain, obj.Body, m.Key)
		_ = obj.Body.Close()
		if err != nil {
			return n, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return n, err
		}
		if err := os.WriteFile(dst, plain.Bytes(), 0o600); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
