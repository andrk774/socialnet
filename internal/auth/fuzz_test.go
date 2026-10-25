package auth

import (
	"strings"
	"testing"
	"time"
)

func FuzzVerifyPassword(f *testing.F) {
	h, _ := HashPassword("x")
	f.Add("x", h)
	f.Add("", "$argon2id$v=19$m=1,t=1,p=1$$")
	f.Add("x", "$argon2id$v=19$m=4294967295,t=4294967295,p=255$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA")
	f.Fuzz(func(t *testing.T, pw, encoded string) {
		// не паникует и не уходит в огромные параметры Argon2 (они ограничены)
		_, _ = VerifyPassword(pw, encoded)
	})
}

func FuzzParseAccess(f *testing.F) {
	ti := NewTokenIssuer([]byte(strings.Repeat("k", 32)), time.Minute)
	tok, _, _ := ti.IssueAccess(1, "s")
	f.Add(tok)
	f.Fuzz(func(t *testing.T, s string) {
		if id, _, err := ti.ParseAccess(s); err == nil && s != tok && id != 1 {
			t.Fatalf("forged token accepted: %q", s)
		}
	})
}
