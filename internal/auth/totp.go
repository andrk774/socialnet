package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP (RFC 6238) по стандарту использует HMAC-SHA1
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ---------- TOTP (RFC 6238), совместим с Google Authenticator ----------

const (
	totpPeriod = 30
	totpDigits = 6
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func NewTOTPSecret() (string, error) {
	b := make([]byte, 20) // 160 бит, как рекомендует RFC 4226
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func TOTPURI(issuer, account, secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}

func hotp(key []byte, counter uint64) string {
	mac := hmac.New(sha1.New, key)
	_ = binary.Write(mac, binary.BigEndian, counter)
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1_000_000)
}

// TOTPCode — код для момента t (нужен тестам).
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	return hotp(key, uint64(t.Unix()/totpPeriod)), nil
}

// VerifyTOTP проверяет код с окном ±1 шаг (часы телефона могут отставать).
// lastStep — последний принятый шаг: повторно тот же код не пройдёт
// (защита от перехвата и повторного использования кода).
// Возвращает шаг, на котором код совпал.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for _, step := range []int64{cur - 1, cur, cur + 1} {
		if step <= lastStep {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(key, uint64(step))), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// ---------- резервные коды ----------

const backupAlphabet = "abcdefghjkmnpqrstuvwxyz23456789" // без похожих символов (l/1, o/0)

// NewBackupCodes возвращает 10 кодов вида "abcd-efgh" и их хэши для базы.
func NewBackupCodes() (codes []string, hashes [][]byte, err error) {
	for i := 0; i < 10; i++ {
		var sb strings.Builder
		buf := make([]byte, 1)
		for n := 0; n < 8; {
			if _, err = rand.Read(buf); err != nil {
				return nil, nil, err
			}
			// отбрасываем значения, дающие перекос при делении по модулю
			if int(buf[0]) >= 256-256%len(backupAlphabet) {
				continue
			}
			if n == 4 {
				sb.WriteByte('-')
			}
			sb.WriteByte(backupAlphabet[int(buf[0])%len(backupAlphabet)])
			n++
		}
		c := sb.String()
		codes = append(codes, c)
		hashes = append(hashes, HashBackupCode(c))
	}
	return codes, hashes, nil
}

func HashBackupCode(code string) []byte {
	c := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	h := sha256.Sum256([]byte("backup:" + c))
	return h[:]
}

// ---------- шифрование секретов в базе (AES-256-GCM) ----------

type SecretBox struct{ aead cipher.AEAD }

func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, errors.New("secret box key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (b *SecretBox) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

func (b *SecretBox) Open(sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("sealed data too short")
	}
	return b.aead.Open(nil, sealed[:n], sealed[n:], nil)
}

// ---------- одноразовые токены из писем ----------

// NewEmailToken: токен для ссылки в письме и его хэш для базы.
func NewEmailToken() (string, []byte, error) { return NewRefreshToken() }
