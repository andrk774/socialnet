package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestUpgradeFrom0004: база версии 4 с живыми данными обновляется без потерь.
func TestUpgradeFrom0004(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	const name = "socialnet_upgrade_test"
	admin.Exec(ctx, `DROP DATABASE IF EXISTS `+name) //nolint:errcheck
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`) //nolint:errcheck

	cfg, _ := pgxpool.ParseConfig(url)
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := migrateUpTo(ctx, pool, "0004_media_wallet_presence"); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO users (email, username, password_hash) VALUES ('a@x.ru','anna','h'), ('b@x.ru','bob','h')`)
	exec(`INSERT INTO communities (slug, name, owner_id) VALUES ('old_club', 'Клуб', 1)`)
	exec(`INSERT INTO posts (author_id, text, likes_count, comments_count) VALUES (1, 'старый пост', 1, 1)`)
	exec(`INSERT INTO post_likes (post_id, user_id) VALUES (1, 2)`)
	exec(`INSERT INTO comments (post_id, author_id, text) VALUES (1, 2, 'старый комментарий')`)
	exec(`INSERT INTO conversations (user_a, user_b) VALUES (1, 2)`)
	exec(`INSERT INTO conversation_state (conversation_id, user_id) VALUES (1, 1)`) // вторая строка «потеряна»
	exec(`INSERT INTO messages (conversation_id, sender_id, body) VALUES (1, 1, 'привет')`)
	exec(`INSERT INTO messages (conversation_id, sender_id, ciphertext, nonce) VALUES (1, 2, '\x01', '\x02')`)

	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var reaction, kind, convKind, vis string
	var states, replies int
	err = pool.QueryRow(ctx, `
		SELECT (SELECT reaction FROM post_likes), (SELECT string_agg(kind, ',' ORDER BY id) FROM messages),
		       (SELECT kind FROM conversations), (SELECT visibility FROM communities),
		       (SELECT count(*) FROM conversation_state), (SELECT replies_count FROM comments)`).
		Scan(&reaction, &kind, &convKind, &vis, &states, &replies)
	if err != nil {
		t.Fatal(err)
	}
	if reaction != "like" || kind != "text,text" || convKind != "direct" || vis != "open" || states != 2 || replies != 0 {
		t.Fatalf("after upgrade: %s %s %s %s states=%d replies=%d", reaction, kind, convKind, vis, states, replies)
	}
	// старые ограничения заменены новыми, а не просто сняты
	if _, err := pool.Exec(ctx, `INSERT INTO messages (conversation_id, sender_id, body, ciphertext) VALUES (1, 1, 'x', '\x01')`); err == nil {
		t.Fatal("body+ciphertext accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversations (kind, user_a, user_b) VALUES ('group', 1, 2)`); err == nil {
		t.Fatal("group with user_a/user_b accepted")
	}
	// повторный запуск — ничего не делает
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
