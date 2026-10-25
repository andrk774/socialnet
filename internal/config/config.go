// Package config читает настройки из переменных окружения.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	JWTSecret    []byte
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	CookieSecure bool // false только для локальной разработки по http://
	TrustProxy   bool // true, если API стоит за Nginx и ему можно верить в X-Forwarded-For

	// Лимит на /api/v1/auth/* с одного IP: запросов в минуту и размер всплеска.
	AuthRatePerMin int
	AuthRateBurst  int

	// Сайты, с которых браузеру разрешено открывать WebSocket
	// (например "app.example.com"). Пусто = только тот же домен, что у API.
	AllowedOrigins []string

	MediaDir string // куда сохранять загруженные фото

	// Почта. Пустой SMTPHost = письма пишутся в лог (режим разработки).
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	MailFrom     string
	AppURL       string // адрес фронтенда для ссылок в письмах

	// Ключ AES-256 для шифрования секретов 2FA в базе (32 байта).
	TOTPKey []byte

	// Адрес для сообщений об уязвимостях (security.txt).
	SecurityContact string

	// Ключи VAPID для Web Push (команда `api gen-vapid`). Пусто = push выключен.
	VAPIDPublic  string
	VAPIDPrivate string
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		MediaDir:     getenv("MEDIA_DIR", "./data/media"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		JWTSecret:    []byte(os.Getenv("JWT_SECRET")),
		CookieSecure: true,
	}

	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 bytes (generate: openssl rand -hex 32)")
	}

	var err error
	if c.AccessTTL, err = duration("ACCESS_TOKEN_TTL", 15*time.Minute); err != nil {
		return c, err
	}
	if c.RefreshTTL, err = duration("REFRESH_TOKEN_TTL", 30*24*time.Hour); err != nil {
		return c, err
	}
	if c.CookieSecure, err = boolean("COOKIE_SECURE", true); err != nil {
		return c, err
	}
	if c.TrustProxy, err = boolean("TRUST_PROXY", false); err != nil {
		return c, err
	}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	c.SMTPHost = os.Getenv("SMTP_HOST")
	c.SMTPPort = getenv("SMTP_PORT", "587")
	c.SMTPUser = os.Getenv("SMTP_USER")
	c.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	c.MailFrom = getenv("MAIL_FROM", "Socialnet <no-reply@localhost>")
	c.SecurityContact = getenv("SECURITY_CONTACT", "security@localhost")
	c.VAPIDPublic = os.Getenv("VAPID_PUBLIC_KEY")
	c.VAPIDPrivate = os.Getenv("VAPID_PRIVATE_KEY")
	c.AppURL = strings.TrimRight(getenv("APP_URL", "http://localhost:3000"), "/")

	if k := os.Getenv("TOTP_ENC_KEY"); k != "" {
		key, err := hex.DecodeString(k)
		if err != nil || len(key) != 32 {
			return c, errors.New("TOTP_ENC_KEY must be 64 hex characters (openssl rand -hex 32)")
		}
		c.TOTPKey = key
	} else {
		// Ключ выводится из JWT_SECRET. Удобно, но при смене JWT_SECRET
		// все настроенные 2FA перестанут работать. Для продакшена задайте свой.
		h := sha256.Sum256(append([]byte("socialnet-totp-key-v1:"), c.JWTSecret...))
		c.TOTPKey = h[:]
	}

	if c.AuthRatePerMin, err = integer("AUTH_RATE_PER_MIN", 10); err != nil {
		return c, err
	}
	if c.AuthRateBurst, err = integer("AUTH_RATE_BURST", 20); err != nil {
		return c, err
	}
	return c, nil
}

func integer(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func duration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func boolean(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}
