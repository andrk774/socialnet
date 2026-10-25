package s3test

import (
	"io"
	"testing"
)

func TestClientAgainstFake(t *testing.T) {
	s := New(t)
	ctx := t.Context()
	c := s.Client
	if err := c.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(ctx, "dir/файл 1.txt", []byte("привет, мир"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	obj, err := c.Get(ctx, "dir/файл 1.txt", "bytes=0-11")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(obj.Body)
	obj.Body.Close()
	if obj.Status != 206 || string(b) != "привет" {
		t.Fatalf("%d %q", obj.Status, b)
	}
	keys, err := c.List(ctx, "dir/")
	if err != nil || len(keys) != 1 || keys[0] != "dir/файл 1.txt" {
		t.Fatalf("list: %v %v", keys, err)
	}
	if err := c.Delete(ctx, keys[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, keys[0], ""); err == nil {
		t.Fatal("deleted object still readable")
	}
	bad := *c
	bad.SecretKey = "wrong"
	if err := bad.Put(ctx, "x", []byte("x"), "text/plain"); err == nil || s.BadSigs != 1 {
		t.Fatalf("bad signature accepted: %v", err)
	}
}
