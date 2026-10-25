package redisx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opt, err := ParseURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := New(opt)
	t.Cleanup(c.Close)
	if _, err := c.Do(t.Context(), "FLUSHDB"); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseURL(t *testing.T) {
	o, err := ParseURL("redis://:s3cret@cache:6380/2")
	if err != nil || o.Addr != "cache:6380" || o.Password != "s3cret" || o.DB != 2 || o.TLS {
		t.Fatalf("%+v %v", o, err)
	}
	o, _ = ParseURL("rediss://user:pw@cache")
	if o.Addr != "cache:6379" || o.Username != "user" || o.Password != "pw" || !o.TLS {
		t.Fatalf("%+v", o)
	}
	for _, bad := range []string{"http://x", "redis://", "redis://h/abc"} {
		if _, err := ParseURL(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestReplyParser(t *testing.T) {
	in := "+OK\r\n-ERR boom\r\n:42\r\n$5\r\nhello\r\n$-1\r\n*2\r\n$1\r\na\r\n:1\r\n$999999999\r\n"
	r := bufio.NewReader(strings.NewReader(in))
	want := []string{"OK", "ERR", "42", "hello", "<nil>", "[a 1]", "bad"}
	for i, w := range want {
		v, err := readReply(r)
		got := ""
		switch {
		case err != nil && strings.Contains(err.Error(), "boom"):
			got = "ERR"
		case err != nil:
			got = "bad"
		case v == nil:
			got = "<nil>"
		default:
			got = fmt.Sprint(v)
		}
		if got != w {
			t.Errorf("reply %d: got %q want %q", i, got, w)
		}
	}
}

func TestCommandsAndPubSub(t *testing.T) {
	c := testClient(t)
	ctx := t.Context()
	if _, err := c.Do(ctx, "SET", "k", "значение", "EX", 10); err != nil {
		t.Fatal(err)
	}
	if v, err := c.String(ctx, "GET", "k"); err != nil || v != "значение" {
		t.Fatalf("GET: %q %v", v, err)
	}
	if _, err := c.String(ctx, "GET", "missing"); !errors.Is(err, ErrNil) {
		t.Fatalf("missing key: %v", err)
	}
	if n, err := c.Int(ctx, "INCR", "n"); err != nil || n != 1 {
		t.Fatalf("INCR: %d %v", n, err)
	}
	if _, err := c.Do(ctx, "NOSUCHCOMMAND"); err == nil {
		t.Fatal("error reply not returned")
	}
	if err := c.Ping(ctx); err != nil { // соединение после ошибки Redis живо
		t.Fatal(err)
	}

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	msgs := c.Subscribe(sctx, "chan")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if n, _ := c.Int(ctx, "PUBLISH", "chan", "привет"); n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscriber never attached")
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case m := <-msgs:
		if m.Channel != "chan" || m.Payload != "привет" {
			t.Fatalf("message: %+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message")
	}
	cancel()
	for range msgs { // канал закрывается после отмены
	}
}
