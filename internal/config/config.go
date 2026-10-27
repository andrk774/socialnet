// Package config читает настройки из переменных окружения.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
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
	// TrustedProxies — подсети своих прокси (CIDR). IP клиента — первый справа
	// адрес в X-Forwarded-For не из этих подсетей. Пусто — ровно один прокси
	// (Nginx перед API): берётся последний адрес, который он сам дописал.
	TrustedProxies []string

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

	// Оператор персональных данных — подставляется в юридические документы.
	LegalOperator string // «ИП Иванов Иван Иванович» или «ООО „…“»
	LegalINN      string
	LegalAddress  string
	LegalEmail    string // для запросов субъектов ПДн и отзыва согласия

	// Ключи VAPID для Web Push (команда `api gen-vapid`). Пусто = push выключен.
	VAPIDPublic  string
	VAPIDPrivate string

	DocsDisabled bool // DOCS_DISABLED=true — не показывать /docs

	// ЮKassa: покупка монет. Пустой YOOKASSA_SHOP_ID — покупка выключена.
	YooKassaShopID    string
	YooKassaSecretKey string
	YooKassaAPIURL    string // по умолчанию https://api.yookassa.ru/v3
	YooKassaVATCode   int    // код ставки НДС в чеке: 1 — без НДС, 11 — 22%, 12 — 22/122
	// Принимать уведомления только с адресов ЮKassa. Выключать — только если
	// перед API стоит прокси, который не передаёт реальный IP (TRUST_PROXY).
	YooKassaCheckIP bool

	// S3/MinIO для фото, музыки, голосовых и видео. Пустой S3_ENDPOINT — локальный диск.
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3PathStyle bool
	// MEDIA_CDN_URL — CDN перед бакетом: публичные файлы отдаются редиректом туда.
	MediaCDNURL string

	// TILE_URL — источник тайлов карты, шаблон с {z} {x} {y}. Пусто — карта выключена.
	// Тайлы идут через свой прокси с кешем: посетители не обращаются к чужому серверу.
	TileURL         string
	TileAttribution string
	TileMaxZoom     int

	// Яндекс SmartCaptcha: ключи из консоли Yandex Cloud. Пусто — выключена.
	CaptchaSiteKey, CaptchaSecret, CaptchaURL string

	// PLUS_PRICE — цена подписки «Плюс» в монетах за 30 дней.
	PlusPrice int64

	// WEATHER_URL — прогноз (API в формате Open-Meteo). Пусто — выключен.
	WeatherURL string

	// /metrics для Prometheus — на отдельном внутреннем адресе.
	MetricsAddr  string // по умолчанию :9090; "off" — выключить
	MetricsToken string // необязательный Bearer-токен

	// Резервные копии (`api backup-loop`).
	BackupDir string // папка для копий; "" — только S3
	// Внешнее хранилище для копий (другой поставщик или регион): если сервер
	// пропадёт вместе с дисками, копии и медиафайлы останутся там.
	OffsiteEndpoint, OffsiteRegion, OffsiteBucket, OffsiteAccessKey, OffsiteSecretKey string
	OffsitePathStyle                                                                  bool
	BackupMedia                                                                       bool // шифрованная копия папки медиа (если медиа не в S3)

	BackupKey  []byte // BACKUP_ENC_KEY, 64 hex-символа: шифрование AES-256-GCM
	BackupKeep int
	BackupHour int  // час запуска, UTC
	BackupS3   bool // класть копии в S3 (backups/)

	// REDIS_URL — общие лимиты, тикеты WebSocket и события между экземплярами API.
	RedisURL string
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
	for _, cidr := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if cidr = strings.TrimSpace(cidr); cidr == "" {
			continue
		}
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return c, fmt.Errorf("TRUSTED_PROXIES: %q is not a CIDR (example: 10.0.0.0/8)", cidr)
		}
		c.TrustedProxies = append(c.TrustedProxies, cidr)
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
	c.LegalOperator = strings.TrimSpace(os.Getenv("LEGAL_OPERATOR"))
	c.LegalINN = strings.TrimSpace(os.Getenv("LEGAL_INN"))
	c.LegalAddress = strings.TrimSpace(os.Getenv("LEGAL_ADDRESS"))
	c.LegalEmail = strings.TrimSpace(os.Getenv("LEGAL_EMAIL"))
	if c.DocsDisabled, err = boolean("DOCS_DISABLED", false); err != nil {
		return c, err
	}
	c.VAPIDPublic = os.Getenv("VAPID_PUBLIC_KEY")
	c.VAPIDPrivate = os.Getenv("VAPID_PRIVATE_KEY")
	c.AppURL = strings.TrimRight(getenv("APP_URL", "http://localhost:3000"), "/")

	c.RedisURL = os.Getenv("REDIS_URL")
	c.MetricsAddr = getenv("METRICS_ADDR", ":9090")
	c.BackupDir = getenv("BACKUP_DIR", "")
	if k := os.Getenv("BACKUP_ENC_KEY"); k != "" {
		key, err := hex.DecodeString(k)
		if err != nil || len(key) != 32 {
			return c, errors.New("BACKUP_ENC_KEY must be 64 hex characters (openssl rand -hex 32)")
		}
		c.BackupKey = key
	}
	if c.BackupKeep, err = strconv.Atoi(getenv("BACKUP_KEEP", "14")); err != nil || c.BackupKeep < 1 {
		return c, errors.New("BACKUP_KEEP must be a positive number")
	}
	if c.BackupHour, err = strconv.Atoi(getenv("BACKUP_HOUR", "3")); err != nil || c.BackupHour < 0 || c.BackupHour > 23 {
		return c, errors.New("BACKUP_HOUR must be 0..23")
	}
	if c.BackupS3, err = boolean("BACKUP_S3", false); err != nil {
		return c, err
	}
	c.OffsiteEndpoint = strings.TrimRight(os.Getenv("BACKUP_S3_ENDPOINT"), "/")
	c.OffsiteRegion = getenv("BACKUP_S3_REGION", "ru-1")
	c.OffsiteBucket = os.Getenv("BACKUP_S3_BUCKET")
	c.OffsiteAccessKey = os.Getenv("BACKUP_S3_ACCESS_KEY")
	c.OffsiteSecretKey = os.Getenv("BACKUP_S3_SECRET_KEY")
	if c.OffsitePathStyle, err = boolean("BACKUP_S3_PATH_STYLE", true); err != nil {
		return c, err
	}
	if c.BackupMedia, err = boolean("BACKUP_MEDIA", true); err != nil {
		return c, err
	}
	if c.OffsiteEndpoint != "" {
		if c.OffsiteBucket == "" || c.OffsiteAccessKey == "" || c.OffsiteSecretKey == "" {
			return c, errors.New("BACKUP_S3_BUCKET, BACKUP_S3_ACCESS_KEY and BACKUP_S3_SECRET_KEY are required with BACKUP_S3_ENDPOINT")
		}
		if !strings.HasPrefix(c.OffsiteEndpoint, "https://") && !strings.HasPrefix(c.OffsiteEndpoint, "http://") {
			return c, errors.New("BACKUP_S3_ENDPOINT must be an http(s) URL")
		}
		if os.Getenv("BACKUP_ENC_KEY") == "" {
			return c, errors.New("BACKUP_S3_ENDPOINT requires BACKUP_ENC_KEY: copies must not leave the server unencrypted")
		}
	}
	c.MetricsToken = os.Getenv("METRICS_TOKEN")
	c.S3Endpoint = strings.TrimRight(os.Getenv("S3_ENDPOINT"), "/")
	c.S3Region = getenv("S3_REGION", "us-east-1")
	c.S3Bucket = os.Getenv("S3_BUCKET")
	c.S3AccessKey = os.Getenv("S3_ACCESS_KEY")
	c.S3SecretKey = os.Getenv("S3_SECRET_KEY")
	if c.S3PathStyle, err = boolean("S3_PATH_STYLE", true); err != nil {
		return c, err
	}
	if c.S3Endpoint != "" && (c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "") {
		return c, errors.New("S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY are required with S3_ENDPOINT")
	}
	c.MediaCDNURL = strings.TrimRight(os.Getenv("MEDIA_CDN_URL"), "/")
	// задана, но пустая (TILE_URL=) — карта выключена; не задана — OpenStreetMap
	c.TileURL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	if v, set := os.LookupEnv("TILE_URL"); set {
		c.TileURL = strings.TrimSpace(v)
	}
	if c.TileURL != "" && (!strings.HasPrefix(c.TileURL, "https://") && !strings.HasPrefix(c.TileURL, "http://") ||
		!strings.Contains(c.TileURL, "{z}") || !strings.Contains(c.TileURL, "{x}") || !strings.Contains(c.TileURL, "{y}")) {
		return c, errors.New("TILE_URL must be an http(s) URL template with {z}, {x} and {y}")
	}
	c.TileAttribution = getenv("TILE_ATTRIBUTION", "© участники OpenStreetMap")
	c.CaptchaSiteKey = strings.TrimSpace(os.Getenv("CAPTCHA_SITE_KEY"))
	c.CaptchaSecret = strings.TrimSpace(os.Getenv("CAPTCHA_SECRET"))
	c.CaptchaURL = getenv("CAPTCHA_URL", "https://smartcaptcha.cloud.yandex.ru/validate")
	if (c.CaptchaSiteKey == "") != (c.CaptchaSecret == "") {
		return c, errors.New("set both CAPTCHA_SITE_KEY and CAPTCHA_SECRET, or neither")
	}
	c.PlusPrice = 199
	if v := os.Getenv("PLUS_PRICE"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 || n > 1_000_000 {
			return c, errors.New("PLUS_PRICE must be 1..1000000 coins")
		}
		c.PlusPrice = n
	}
	c.WeatherURL = "https://api.open-meteo.com/v1/forecast"
	if v, set := os.LookupEnv("WEATHER_URL"); set {
		c.WeatherURL = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	if c.WeatherURL != "" && !strings.HasPrefix(c.WeatherURL, "https://") && !strings.HasPrefix(c.WeatherURL, "http://") {
		return c, errors.New("WEATHER_URL must be an http(s) URL")
	}
	c.TileMaxZoom = 17
	if v := os.Getenv("TILE_MAX_ZOOM"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 20 {
			return c, errors.New("TILE_MAX_ZOOM must be 1-20")
		}
		c.TileMaxZoom = n
	}
	if c.BackupS3 && c.S3Endpoint == "" {
		return c, errors.New("BACKUP_S3=true requires S3_ENDPOINT")
	}
	c.YooKassaShopID = os.Getenv("YOOKASSA_SHOP_ID")
	c.YooKassaSecretKey = os.Getenv("YOOKASSA_SECRET_KEY")
	c.YooKassaAPIURL = strings.TrimRight(getenv("YOOKASSA_API_URL", "https://api.yookassa.ru/v3"), "/")
	if c.YooKassaShopID != "" && c.YooKassaSecretKey == "" {
		return c, errors.New("YOOKASSA_SECRET_KEY is required when YOOKASSA_SHOP_ID is set")
	}
	vat, err := strconv.Atoi(getenv("YOOKASSA_VAT_CODE", "1"))
	if err != nil || vat < 1 || vat > 12 {
		return c, errors.New("YOOKASSA_VAT_CODE must be 1..12 (1 — без НДС, 11 — НДС 22%)")
	}
	c.YooKassaVATCode = vat
	if c.YooKassaCheckIP, err = boolean("YOOKASSA_CHECK_IP", true); err != nil {
		return c, err
	}

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
