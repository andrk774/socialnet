package backup

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"socialnet/internal/s3/s3test"
)

func TestMediaSync(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	write := func(rel, data string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ab/abcd.jpg", "секретное фото")
	write("routes/r1/route.gpx", "<gpx/>")
	write("routes/r1/.upload-123", "недогружено")
	write("tile-cache/1/0/0.tile", "кеш карты")
	fake := s3test.New(t)
	key := bytes.Repeat([]byte{3}, 32)
	m := MediaSync{Dir: dir, S3: fake.Client, Prefix: "media/", Key: key, Log: slog.Default()}

	if _, err := (MediaSync{Dir: dir, S3: fake.Client, Prefix: "media/", Log: slog.Default()}).Run(ctx); err == nil {
		t.Fatal("unencrypted media backup allowed")
	}
	res, err := m.Run(ctx)
	if err != nil || res.Uploaded != 2 || res.Total != 2 {
		t.Fatalf("first sync: %+v %v", res, err)
	}
	if keys := strings.Join(fake.Keys(), " "); keys != "media/ab/abcd.jpg.enc media/routes/r1/route.gpx.enc" {
		t.Fatalf("keys: %s", keys)
	}
	obj, _ := fake.Client.Get(ctx, "media/ab/abcd.jpg.enc", "")
	var raw bytes.Buffer
	_, _ = raw.ReadFrom(obj.Body)
	if bytes.Contains(raw.Bytes(), []byte("секретное")) {
		t.Fatal("media stored unencrypted")
	}
	// повтор ничего не грузит; удалённый на сервере файл пропадает и из копии
	if res, _ := m.Run(ctx); res.Uploaded != 0 {
		t.Fatalf("resync uploaded again: %+v", res)
	}
	_ = os.Remove(filepath.Join(dir, "routes/r1/route.gpx"))
	if res, err := m.Run(ctx); err != nil || res.Deleted != 1 {
		t.Fatalf("deletion not propagated: %+v %v", res, err)
	}

	// восстановление на «новый сервер»
	dst := t.TempDir()
	n, err := MediaSync{Dir: dst, S3: fake.Client, Prefix: "media/", Key: key}.Restore(ctx)
	if err != nil || n != 1 {
		t.Fatalf("restore: %d %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "ab/abcd.jpg")); string(b) != "секретное фото" {
		t.Fatalf("restored: %q", b)
	}
	// ключ с «..» из подменённого хранилища не выводит за пределы папки
	_ = fake.Client.Put(ctx, "media/../../evil.enc", []byte("x"), "application/octet-stream")
	if _, err := (MediaSync{Dir: t.TempDir(), S3: fake.Client, Prefix: "media/", Key: key}).Restore(ctx); err == nil {
		t.Fatal("path traversal key accepted")
	}
}
