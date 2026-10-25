package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"socialnet/internal/db"
	"socialnet/internal/s3/s3test"
)

func TestEncryptRoundTripAndTamper(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	for _, size := range []int{0, 1, chunkSize - 1, chunkSize, chunkSize + 1, 3*chunkSize + 17} {
		plain := make([]byte, size)
		_, _ = rand.Read(plain)
		var enc, dec bytes.Buffer
		if err := Encrypt(&enc, bytes.NewReader(plain), key); err != nil {
			t.Fatal(err)
		}
		if err := Decrypt(&dec, bytes.NewReader(enc.Bytes()), key); err != nil || !bytes.Equal(dec.Bytes(), plain) {
			t.Fatalf("size %d: round trip failed: %v", size, err)
		}
		e := enc.Bytes()
		// обрезанный файл (атакующий отрезал последние блоки) не принимается
		if size > chunkSize {
			cut := len(magic) + 8 + 4 + chunkSize + 16
			if err := Decrypt(discard{}, bytes.NewReader(e[:cut]), key); err == nil {
				t.Fatalf("size %d: truncated backup accepted", size)
			}
		}
		// испорченный байт и чужой ключ
		bad := append([]byte(nil), e...)
		bad[len(bad)-1] ^= 1
		if err := Decrypt(discard{}, bytes.NewReader(bad), key); err == nil {
			t.Fatalf("size %d: tampered backup accepted", size)
		}
		other := make([]byte, 32)
		if err := Decrypt(discard{}, bytes.NewReader(e), other); err == nil {
			t.Fatalf("size %d: wrong key accepted", size)
		}
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestBackupWithRestoreCheck(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not installed")
	}
	ctx := context.Background()
	// отдельная база, чтобы не мешать тестам API
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	const name = "socialnet_backup_test"
	admin.Exec(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`) //nolint:errcheck
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`) //nolint:errcheck
	srcURL := replaceDB(url, "socialnet_test", name)
	pool, err := pgxpool.New(ctx, srcURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (email, username, password_hash) VALUES ('a@x.ru', 'anna', 'h'), ('b@x.ru', 'bob', 'h')`); err != nil {
		t.Fatal(err)
	}
	pool.Close()

	key := bytes.Repeat([]byte{7}, 32)
	dir := t.TempDir()
	fake := s3test.New(t)
	o := Options{DatabaseURL: srcURL, Dir: dir, S3: fake.Client, S3Prefix: "backups/", Key: key, Keep: 2}
	var names []string
	for i := 0; i < 3; i++ {
		res, err := Run(ctx, o)
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		if !res.RestoreOK || !res.Encrypted || res.Tables < 30 || !strings.HasPrefix(res.Migration, "0008") {
			t.Fatalf("result: %+v", res)
		}
		names = append(names, res.Name)
		if i < 2 {
			// имя копии с точностью до секунды; чтобы ротация видела разные файлы
			os.Rename(filepath.Join(dir, res.Name), filepath.Join(dir, "socialnet-2000010"+string(rune('1'+i))+"-000000.dump.enc")) //nolint:errcheck
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "socialnet-*.dump.enc"))
	if len(files) != 2 {
		t.Fatalf("retention kept %d files: %v", len(files), files)
	}
	if keys := fake.Keys(); len(keys) < 1 || !strings.HasPrefix(keys[0], "backups/socialnet-") {
		t.Fatalf("s3: %v", keys)
	}
	data, _ := os.ReadFile(filepath.Join(dir, names[2]))
	if bytes.Contains(data, []byte("a@x.ru")) {
		t.Fatal("backup is not encrypted")
	}

	// восстановление в новую базу
	const target = "socialnet_restore_target"
	admin.Exec(ctx, `DROP DATABASE IF EXISTS `+target+` WITH (FORCE)`) //nolint:errcheck
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+target); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+target+` WITH (FORCE)`) //nolint:errcheck
	if err := Restore(ctx, filepath.Join(dir, names[2]), replaceDB(url, "socialnet_test", target), key); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, replaceDB(url, "socialnet_test", target))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("restored users: %d %v", n, err)
	}
	// после проверок временные базы удалены
	var left int
	admin.QueryRow(ctx, `SELECT count(*) FROM pg_database WHERE datname LIKE 'socialnet_restore_check_%'`).Scan(&left) //nolint:errcheck
	if left != 0 {
		t.Fatalf("%d scratch databases left", left)
	}
}

func TestReplaceDB(t *testing.T) {
	if got := replaceDB("postgres://u:p@h:5432/socialnet?sslmode=disable", "socialnet", "x"); got != "postgres://u:p@h:5432/x?sslmode=disable" {
		t.Fatal(got)
	}
}
