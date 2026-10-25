package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected format: %s", h)
	}
	if ok, _ := VerifyPassword("correct horse battery staple", h); !ok {
		t.Fatal("correct password rejected")
	}
	if ok, _ := VerifyPassword("wrong", h); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse battery staple")
	if h == h2 {
		t.Fatal("salt not random: identical hashes")
	}
}

func TestVerifyMalformed(t *testing.T) {
	for _, bad := range []string{"", "plain", "$bcrypt$x$y$z$w", "$argon2id$v=19$m=x$a$b"} {
		if _, err := VerifyPassword("x", bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestAccessToken(t *testing.T) {
	ti := NewTokenIssuer([]byte(strings.Repeat("k", 32)), time.Minute)
	tok, _, err := ti.IssueAccess(42, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	id, sid, err := ti.ParseAccess(tok)
	if err != nil || id != 42 || sid != "sess-1" {
		t.Fatalf("got %d %q, %v", id, sid, err)
	}

	other := NewTokenIssuer([]byte(strings.Repeat("z", 32)), time.Minute)
	if _, _, err := other.ParseAccess(tok); err == nil {
		t.Fatal("token signed with another key accepted")
	}

	expired := NewTokenIssuer([]byte(strings.Repeat("k", 32)), -time.Minute)
	old, _, _ := expired.IssueAccess(42, "s")
	if _, _, err := ti.ParseAccess(old); err == nil {
		t.Fatal("expired token accepted")
	}

	// токен второго шага 2FA не годится как access, и наоборот
	mfa, _ := ti.IssueMFA(42)
	if _, _, err := ti.ParseAccess(mfa); err == nil {
		t.Fatal("mfa token accepted as access token")
	}
	if _, err := ti.ParseMFA(tok); err == nil {
		t.Fatal("access token accepted as mfa token")
	}
	if uid, err := ti.ParseMFA(mfa); err != nil || uid != 42 {
		t.Fatal("mfa token rejected")
	}
}

func TestAlgNoneRejected(t *testing.T) {
	ti := NewTokenIssuer([]byte(strings.Repeat("k", 32)), time.Minute)
	claims := jwt.RegisteredClaims{
		Issuer: issuer, Subject: "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ti.ParseAccess(tok); err == nil {
		t.Fatal("alg=none token accepted")
	}
}

func TestRefreshTokenHash(t *testing.T) {
	a, ha, _ := NewRefreshToken()
	b, _, _ := NewRefreshToken()
	if a == b {
		t.Fatal("refresh tokens not unique")
	}
	if string(HashRefreshToken(a)) != string(ha) {
		t.Fatal("hash mismatch")
	}
}

func TestTOTP(t *testing.T) {
	// контрольный вектор из RFC 6238 (SHA1, T=59 -> 94287082, 6 младших цифр)
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	code, _ := TOTPCode(secret, time.Unix(59, 0))
	if code != "287082" {
		t.Fatalf("RFC vector: %s", code)
	}

	now := time.Now()
	s, _ := NewTOTPSecret()
	c, _ := TOTPCode(s, now)
	step, ok := VerifyTOTP(s, c, now, 0)
	if !ok {
		t.Fatal("valid code rejected")
	}
	if _, ok := VerifyTOTP(s, c, now, step); ok {
		t.Fatal("replayed code accepted")
	}
	prev, _ := TOTPCode(s, now.Add(-30*time.Second))
	if _, ok := VerifyTOTP(s, prev, now, 0); !ok {
		t.Fatal("previous step should be accepted (clock drift)")
	}
	old, _ := TOTPCode(s, now.Add(-5*time.Minute))
	if _, ok := VerifyTOTP(s, old, now, 0); ok {
		t.Fatal("old code accepted")
	}
}

func TestBackupCodesAndBox(t *testing.T) {
	codes, hashes, err := NewBackupCodes()
	if err != nil || len(codes) != 10 || len(codes[0]) != 9 {
		t.Fatalf("codes: %v %v", codes, err)
	}
	if string(HashBackupCode(strings.ToUpper(codes[0]))) != string(hashes[0]) {
		t.Fatal("backup code normalization")
	}

	box, _ := NewSecretBox([]byte(strings.Repeat("k", 32)))
	sealed, _ := box.Seal([]byte("JBSWY3DPEHPK3PXP"))
	if strings.Contains(string(sealed), "JBSWY3") {
		t.Fatal("not encrypted")
	}
	plain, err := box.Open(sealed)
	if err != nil || string(plain) != "JBSWY3DPEHPK3PXP" {
		t.Fatal("open failed")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := box.Open(sealed); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
