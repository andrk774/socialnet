package webpush

import (
	"crypto/ecdh"
	"crypto/rand"
	"testing"
)

func FuzzDecrypt(f *testing.F) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	valid, _ := Encrypt([]byte("hello"), ua.PublicKey().Bytes(), auth)
	f.Add(valid)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = Decrypt(b, ua, auth) // не должно паниковать
	})
}

func FuzzValidateEndpoint(f *testing.F) {
	f.Add("https://fcm.googleapis.com/x")
	f.Add("https://169.254.169.254/")
	f.Fuzz(func(t *testing.T, s string) {
		if ValidateEndpoint(s, DefaultHosts) == nil {
			// всё, что прошло проверку, должно быть https на разрешённом хосте
			if len(s) < 8 || s[:8] != "https://" {
				t.Fatalf("non-https endpoint accepted: %q", s)
			}
		}
	})
}
