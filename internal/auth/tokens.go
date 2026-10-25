package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	issuer      = "socialnet"
	audAccess   = "access"
	audMFA      = "mfa"
	mfaTokenTTL = 5 * time.Minute
)

var ErrInvalidToken = errors.New("invalid token")

type TokenIssuer struct {
	secret    []byte
	accessTTL time.Duration
}

func NewTokenIssuer(secret []byte, accessTTL time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: secret, accessTTL: accessTTL}
}

type claims struct {
	jwt.RegisteredClaims
	SID string `json:"sid,omitempty"` // идентификатор сессии
}

func (t *TokenIssuer) issue(userID int64, aud, sid string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(ttl)
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   strconv.FormatInt(userID, 10),
			Audience:  jwt.ClaimStrings{aud},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		SID: sid,
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	return s, exp, err
}

func (t *TokenIssuer) parse(token, aud string) (int64, string, error) {
	c := &claims{}
	_, err := jwt.ParseWithClaims(token, c,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(aud), // токен второго шага нельзя подсунуть вместо обычного
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return 0, "", ErrInvalidToken
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, "", ErrInvalidToken
	}
	return id, c.SID, nil
}

// IssueAccess выпускает короткоживущий JWT (HS256) для API.
func (t *TokenIssuer) IssueAccess(userID int64, sid string) (string, time.Time, error) {
	return t.issue(userID, audAccess, sid, t.accessTTL)
}

// ParseAccess проверяет подпись, срок, издателя и аудиторию. Алгоритм жёстко
// задан как HS256: это закрывает атаки с alg=none и подменой алгоритма.
func (t *TokenIssuer) ParseAccess(token string) (userID int64, sid string, err error) {
	userID, sid, err = t.parse(token, audAccess)
	if err == nil && sid == "" {
		return 0, "", ErrInvalidToken
	}
	return userID, sid, err
}

// IssueMFA — токен «пароль верный, ждём код 2FA». Живёт 5 минут.
func (t *TokenIssuer) IssueMFA(userID int64) (string, error) {
	s, _, err := t.issue(userID, audMFA, "", mfaTokenTTL)
	return s, err
}

func (t *TokenIssuer) ParseMFA(token string) (int64, error) {
	id, _, err := t.parse(token, audMFA)
	return id, err
}

// NewRefreshToken возвращает случайный токен (уходит клиенту в cookie)
// и его SHA-256 (хранится в базе).
func NewRefreshToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashRefreshToken(token), nil
}

func HashRefreshToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
