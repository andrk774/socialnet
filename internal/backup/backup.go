package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"socialnet/internal/s3"
)

type Options struct {
	DatabaseURL string
	Dir         string     // локальная папка для копий ("" — только S3)
	S3          *s3.Client // nil — без S3
	S3Prefix    string     // "backups/"
	Key         []byte     // 32 байта; nil — без шифрования (не рекомендуется)
	Keep        int        // сколько последних копий хранить
	Log         *slog.Logger
}

type Result struct {
	Name       string        `json:"name"`
	Size       int64         `json:"size"`
	SHA256     string        `json:"sha256"`
	Encrypted  bool          `json:"encrypted"`
	RestoreOK  bool          `json:"restore_ok"`
	Tables     int           `json:"tables"`
	Migration  string        `json:"migration"`
	Duration   time.Duration `json:"duration"`
	RestoreErr string        `json:"restore_error,omitempty"`
}

// Таблицы, по которым сверяем копию: в восстановленной базе строк должно
// быть не меньше, чем было при снятии копии (база могла вырасти, но не наоборот).
var checkTables = []string{"users", "posts", "messages", "communities", "wallet_transactions", "audit_log"}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // фиксированные утилиты PostgreSQL, аргументы наши
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Run снимает копию, сохраняет, проверяет восстановлением и чистит старые.
// Копия, которая не восстановилась, считается неудачной.
func Run(ctx context.Context, o Options) (*Result, error) {
	start := time.Now()
	if o.Keep <= 0 {
		o.Keep = 14
	}
	work, err := os.MkdirTemp("", "snbackup-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()

	// счётчики до копии: восстановленная база не может быть «меньше»
	before, migration, err := stats(ctx, o.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("read source stats: %w", err)
	}
	dump := filepath.Join(work, "db.dump")
	if err := run(ctx, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file="+dump, "--dbname="+o.DatabaseURL); err != nil {
		return nil, err
	}

	res := &Result{Name: "socialnet-" + time.Now().UTC().Format("20060102-150405") + ".dump", Migration: migration}
	final := dump
	if o.Key != nil {
		res.Name += ".enc"
		res.Encrypted = true
		final = dump + ".enc"
		if err := encryptFile(dump, final, o.Key); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(final) //nolint:gosec // наш временный файл
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	res.SHA256, res.Size = hex.EncodeToString(sum[:]), int64(len(data))

	// Проверка восстановлением — по сохранённому файлу (с расшифровкой), а
	// не по исходному дампу: проверяем ровно то, что будем восстанавливать.
	if err := restoreCheck(ctx, o, data, before, migration, res); err != nil {
		res.RestoreErr = err.Error()
	} else {
		res.RestoreOK = true
	}

	if o.Dir != "" {
		if err := os.MkdirAll(o.Dir, 0o700); err != nil {
			return res, err
		}
		if err := os.WriteFile(filepath.Join(o.Dir, res.Name), data, 0o600); err != nil { // #nosec G703 -- каталог из настроек, имя файла создаёт сам сервер
			return res, err
		}
		if err := os.WriteFile(filepath.Join(o.Dir, res.Name+".sha256"), []byte(res.SHA256+"  "+res.Name+"\n"), 0o600); err != nil { // #nosec G703 -- то же имя, созданное сервером
			return res, err
		}
		if err := pruneDir(o.Dir, o.Keep); err != nil {
			return res, err
		}
	}
	if o.S3 != nil {
		if err := o.S3.Put(ctx, o.S3Prefix+res.Name, data, "application/octet-stream"); err != nil {
			return res, err
		}
		if err := pruneS3(ctx, o.S3, o.S3Prefix, o.Keep); err != nil {
			return res, err
		}
	}
	res.Duration = time.Since(start)
	if !res.RestoreOK {
		return res, errors.New("restore check failed: " + res.RestoreErr)
	}
	return res, nil
}

func encryptFile(in, out string, key []byte) error {
	src, err := os.Open(in) //nolint:gosec // наш временный файл
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600) //nolint:gosec
	if err != nil {
		return err
	}
	if err := Encrypt(dst, src, key); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

func connect(ctx context.Context, url string) (*pgx.Conn, error) {
	return pgx.Connect(ctx, url)
}

func stats(ctx context.Context, url string) (map[string]int64, string, error) {
	conn, err := connect(ctx, url)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = conn.Close(ctx) }()
	out := map[string]int64{}
	for _, t := range checkTables {
		var n int64
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{t}.Sanitize()).Scan(&n); err != nil {
			return nil, "", err
		}
		out[t] = n
	}
	var mig string
	err = conn.QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&mig)
	return out, mig, err
}

// restoreCheck: временная база → pg_restore → сверка → удаление.
func restoreCheck(ctx context.Context, o Options, data []byte, before map[string]int64, migration string, res *Result) error {
	work, err := os.MkdirTemp("", "snrestore-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	file := filepath.Join(work, "restore.dump")
	if o.Key != nil {
		var plain bytes.Buffer
		if err := Decrypt(&plain, bytes.NewReader(data), o.Key); err != nil {
			return err
		}
		data = plain.Bytes()
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return err
	}

	scratch := "socialnet_restore_check_" + time.Now().UTC().Format("20060102150405")
	admin, err := connect(ctx, o.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{scratch}.Sanitize()); err != nil {
		return fmt.Errorf("create scratch db: %w", err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{scratch}.Sanitize()+" WITH (FORCE)")
	}()
	cfg, err := pgx.ParseConfig(o.DatabaseURL)
	if err != nil {
		return err
	}
	scratchURL := replaceDB(o.DatabaseURL, cfg.Database, scratch)
	if err := run(ctx, "pg_restore", "--no-owner", "--no-privileges", "--exit-on-error", "--dbname="+scratchURL, file); err != nil {
		return err
	}
	after, mig, err := stats(ctx, scratchURL)
	if err != nil {
		return err
	}
	if mig != migration {
		return fmt.Errorf("migration %q in backup, %q in source", mig, migration)
	}
	for t, n := range before {
		if after[t] < n {
			return fmt.Errorf("table %s: %d rows restored, %d expected", t, after[t], n)
		}
	}
	conn, err := connect(ctx, scratchURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	return conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&res.Tables)
}

// replaceDB меняет имя базы в URL postgres://…/<db>?… .
func replaceDB(url, from, to string) string {
	base, query, _ := strings.Cut(url, "?")
	if i := strings.LastIndex(base, "/"+from); i >= 0 && i+len(from)+1 == len(base) {
		base = base[:i] + "/" + to
	}
	if query != "" {
		return base + "?" + query
	}
	return base
}

func isBackup(name string) bool {
	return strings.HasPrefix(name, "socialnet-") && (strings.HasSuffix(name, ".dump") || strings.HasSuffix(name, ".dump.enc"))
}

func pruneDir(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if isBackup(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // имя содержит время — сортировка хронологическая
	for i := 0; i < len(names)-keep; i++ {
		_ = os.Remove(filepath.Join(dir, names[i]))
		_ = os.Remove(filepath.Join(dir, names[i]+".sha256"))
	}
	return nil
}

func pruneS3(ctx context.Context, c *s3.Client, prefix string, keep int) error {
	keys, err := c.List(ctx, prefix)
	if err != nil {
		return err
	}
	var names []string
	for _, k := range keys {
		if isBackup(strings.TrimPrefix(k, prefix)) {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for i := 0; i < len(names)-keep; i++ {
		if err := c.Delete(ctx, names[i]); err != nil {
			return err
		}
	}
	return nil
}

// Restore — восстановление в указанную базу (для ручного использования).
func Restore(ctx context.Context, file, databaseURL string, key []byte) error {
	data, err := os.ReadFile(file) //nolint:gosec // путь задаёт администратор
	if err != nil {
		return err
	}
	if key != nil || strings.HasSuffix(file, ".enc") {
		if key == nil {
			return errors.New("backup is encrypted: set BACKUP_ENC_KEY")
		}
		var plain bytes.Buffer
		if err := Decrypt(&plain, bytes.NewReader(data), key); err != nil {
			return err
		}
		data = plain.Bytes()
	}
	tmp, err := os.CreateTemp("", "snrestore-*.dump")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		return err
	}
	_ = tmp.Close()
	return run(ctx, "pg_restore", "--no-owner", "--no-privileges", "--clean", "--if-exists", "--exit-on-error",
		"--dbname="+databaseURL, tmp.Name())
}
