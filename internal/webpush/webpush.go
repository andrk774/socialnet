// Package webpush: отправка Web Push без сторонних сервисов.
//
//   - payload шифруется по RFC 8291 (aes128gcm): push-сервис браузера
//     (Google, Mozilla, Apple) пересылает сообщение, но прочитать его не может;
//   - сервер подтверждает себя ключами VAPID (RFC 8292).
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"socialnet/internal/safehttp"
)

var (
	ErrGone        = errors.New("subscription expired") // 404/410: подписку надо удалить
	ErrBadEndpoint = errors.New("push endpoint not allowed")
	b64            = base64.RawURLEncoding
)

// ---------- ключи VAPID ----------

type VAPID struct {
	priv    *ecdsa.PrivateKey
	Public  string // base64url, 65 байт — отдаётся браузеру в applicationServerKey
	Subject string // mailto: или https: — контакт для push-сервиса
}

// GenerateVAPID создаёт пару ключей (команда `api gen-vapid`).
func GenerateVAPID() (public, private string, err error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return b64.EncodeToString(k.PublicKey().Bytes()), b64.EncodeToString(k.Bytes()), nil
}

func NewVAPID(public, private, subject string) (*VAPID, error) {
	d, err := b64.DecodeString(private)
	if err != nil || len(d) != 32 {
		return nil, errors.New("VAPID_PRIVATE_KEY must be 32 bytes base64url")
	}
	k, err := ecdh.P256().NewPrivateKey(d)
	if err != nil {
		return nil, err
	}
	if b64.EncodeToString(k.PublicKey().Bytes()) != public {
		return nil, errors.New("VAPID_PUBLIC_KEY does not match VAPID_PRIVATE_KEY")
	}
	pub := k.PublicKey().Bytes()
	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(),
			X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:])},
		D: new(big.Int).SetBytes(d),
	}
	return &VAPID{priv: priv, Public: public, Subject: subject}, nil
}

func (v *VAPID) authHeader(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"aud": u.Scheme + "://" + u.Host,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": v.Subject,
	})
	s, err := tok.SignedString(v.priv)
	if err != nil {
		return "", err
	}
	return "vapid t=" + s + ", k=" + v.Public, nil
}

// ---------- шифрование RFC 8291 ----------

// Encrypt шифрует payload для подписки браузера.
func Encrypt(plain, uaPublic, authSecret []byte) ([]byte, error) {
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return encrypt(plain, uaPublic, authSecret, as, salt)
}

func encrypt(plain, uaPublic, authSecret []byte, as *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("bad p256dh: %w", err)
	}
	shared, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPublic := as.PublicKey().Bytes()

	// IKM = HKDF(auth, ecdh_secret, "WebPush: info" || 0x00 || ua_public || as_public)
	keyInfo := append(append([]byte("WebPush: info\x00"), uaPublic...), asPublic...)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// одна запись: данные + разделитель 0x02 «последняя запись»
	ct := gcm.Seal(nil, nonce, append(append([]byte{}, plain...), 0x02), nil)

	// заголовок: salt(16) | rs(4) | idlen(1) | keyid = as_public(65)
	var out bytes.Buffer
	out.Write(salt)
	_ = binary.Write(&out, binary.BigEndian, uint32(4096))
	out.WriteByte(byte(len(asPublic)))
	out.Write(asPublic)
	out.Write(ct)
	return out.Bytes(), nil
}

// Decrypt — обратная операция (на стороне браузера). Нужна тестам.
func Decrypt(body []byte, ua *ecdh.PrivateKey, authSecret []byte) ([]byte, error) {
	if len(body) < 21 {
		return nil, errors.New("short body")
	}
	salt, idlen := body[:16], int(body[20])
	if len(body) < 21+idlen {
		return nil, errors.New("short body")
	}
	asPub, err := ecdh.P256().NewPublicKey(body[21 : 21+idlen])
	if err != nil {
		return nil, err
	}
	shared, err := ua.ECDH(asPub)
	if err != nil {
		return nil, err
	}
	keyInfo := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asPub.Bytes()...)
	ikm, _ := hkdf.Key(sha256.New, shared, authSecret, string(keyInfo), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	pt, err := gcm.Open(nil, nonce, body[21+idlen:], nil)
	if err != nil {
		return nil, err
	}
	pt = bytes.TrimRight(pt, "\x00")
	if len(pt) == 0 || pt[len(pt)-1] != 0x02 {
		return nil, errors.New("bad padding")
	}
	return pt[:len(pt)-1], nil
}

// ---------- проверка адреса подписки (защита от SSRF) ----------

// Адрес подписки присылает клиент, а сервер потом делает на него POST.
// Без проверки это SSRF: можно заставить сервер стучаться во внутреннюю сеть
// (http://169.254.169.254/, http://db:5432/ ...). Поэтому разрешены только
// настоящие push-сервисы браузеров и только https.
var DefaultHosts = []string{
	"fcm.googleapis.com",                // Chrome, Edge, Яндекс.Браузер
	"updates.push.services.mozilla.com", // Firefox
	"web.push.apple.com",                // Safari
	".notify.windows.com",               // старый Edge
}

func ValidateEndpoint(endpoint string, allowed []string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return ErrBadEndpoint
	}
	if p := u.Port(); p != "" && p != "443" && !allowAnyPort(allowed) {
		return ErrBadEndpoint
	}
	host := strings.ToLower(u.Hostname())
	for _, a := range allowed {
		if host == a || (strings.HasPrefix(a, ".") && strings.HasSuffix(host, a)) {
			return nil
		}
	}
	return ErrBadEndpoint
}

// allowAnyPort — только для тестов, где push-сервис поднят на случайном порту.
func allowAnyPort(allowed []string) bool {
	for _, a := range allowed {
		if a == "127.0.0.1" {
			return true
		}
	}
	return false
}

// SafeClient — HTTP-клиент, который не подключается к внутренним адресам,
// даже если DNS push-сервиса вдруг укажет туда (защита от DNS rebinding).
func SafeClient() *http.Client { return safehttp.Client(15 * time.Second) }

// ---------- отправка ----------

type Subscription struct {
	Endpoint string
	P256dh   []byte
	Auth     []byte
}

func (v *VAPID) Send(ctx context.Context, client *http.Client, sub Subscription, payload []byte, ttl time.Duration) error {
	body, err := Encrypt(payload, sub.P256dh, sub.Auth)
	if err != nil {
		return err
	}
	auth, err := v.authHeader(sub.Endpoint)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", fmt.Sprint(int(ttl.Seconds())))
	req.Header.Set("Urgency", "normal")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode >= 300:
		return fmt.Errorf("push service responded %d", resp.StatusCode)
	}
	return nil
}
