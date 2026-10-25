package backup

import (
	"bytes"
	"testing"
)

func FuzzDecrypt(f *testing.F) {
	key := bytes.Repeat([]byte{1}, 32)
	var enc bytes.Buffer
	_ = Encrypt(&enc, bytes.NewReader([]byte("hello backup")), key)
	f.Add(enc.Bytes())
	f.Add([]byte("SNBK1"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var out bytes.Buffer
		if err := Decrypt(&out, bytes.NewReader(data), key); err == nil && !bytes.Equal(data, enc.Bytes()) && out.Len() > 0 {
			// подделать шифртекст без ключа нельзя: успех только у настоящих копий
			if !bytes.Equal(out.Bytes(), []byte("hello backup")) {
				t.Fatalf("forged ciphertext decrypted to %q", out.Bytes())
			}
		}
	})
}
