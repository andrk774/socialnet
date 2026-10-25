// Package store: доступ к данным. Все запросы параметризованы ($1, $2...),
// строки никогда не склеиваются с пользовательским вводом.
package store

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrUsernameTaken = errors.New("username already taken")
	ErrSlugTaken     = errors.New("slug already taken")
	ErrTokenInvalid  = errors.New("refresh token invalid or expired")
	ErrTokenReused   = errors.New("refresh token reuse detected")
	ErrBadMedia      = errors.New("media not found or not owned by user")
	ErrConflict      = errors.New("conflict")
	ErrLimit         = errors.New("limit reached")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool нужен тестам и фоновым задачам.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ---------- пользователи ----------

type User struct {
	ID              int64
	Email           string
	Username        string
	FirstName       string
	LastName        string
	CreatedAt       time.Time
	PasswordHash    string
	StatusText      string
	City            string
	About           string
	AvatarID        *string
	EmailVerifiedAt *time.Time
	Role            string
	BannedAt        *time.Time
	BanReason       string

	ProfileVisibility string
	FriendsVisibility string
	MessagePrivacy    string
	Searchable        bool

	TOTPSecretEnc []byte
	TOTPEnabledAt *time.Time
	TOTPLastStep  int64

	LastSeenAt       *time.Time
	OnlineVisibility string
}

const userCols = `id, email, username, first_name, last_name, created_at, password_hash,
	status_text, city, about, avatar_media_id::text, email_verified_at, role, banned_at, ban_reason,
	profile_visibility, friends_visibility, message_privacy, searchable,
	totp_secret_enc, totp_enabled_at, totp_last_step, last_seen_at, online_visibility`

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.CreatedAt, &u.PasswordHash,
		&u.StatusText, &u.City, &u.About, &u.AvatarID, &u.EmailVerifiedAt, &u.Role, &u.BannedAt, &u.BanReason,
		&u.ProfileVisibility, &u.FriendsVisibility, &u.MessagePrivacy, &u.Searchable,
		&u.TOTPSecretEnc, &u.TOTPEnabledAt, &u.TOTPLastStep, &u.LastSeenAt, &u.OnlineVisibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func isUnique(err error) (*pgconn.PgError, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr, true
	}
	return nil, false
}

func (s *Store) CreateUser(ctx context.Context, email, username, passwordHash, first, last string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, first_name, last_name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userCols,
		email, username, passwordHash, first, last))

	if pgErr, ok := isUnique(err); ok {
		switch pgErr.ConstraintName {
		case "users_email_key":
			return nil, ErrEmailTaken
		case "users_username_key":
			return nil, ErrUsernameTaken
		}
	}
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email))
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE username = $1`, username))
}

// AuthInfo — минимум, нужный на каждый запрос: роль, бан и жива ли сессия.
func (s *Store) AuthInfo(ctx context.Context, id int64, sid string) (role string, banned, sessionOK bool, err error) {
	var bannedAt *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT u.role, u.banned_at,
		       EXISTS (SELECT 1 FROM refresh_tokens t
		               WHERE t.user_id = u.id AND t.session_id = $2::uuid
		                 AND t.revoked_at IS NULL AND t.expires_at > now())
		FROM users u WHERE u.id = $1`, id, sid).Scan(&role, &bannedAt, &sessionOK)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, ErrNotFound
	}
	return role, bannedAt != nil, sessionOK, err
}

type ProfileUpdate struct {
	FirstName  *string
	LastName   *string
	StatusText *string
	City       *string
	About      *string
}

// UpdateProfile меняет только переданные поля (nil = не трогать).
func (s *Store) UpdateProfile(ctx context.Context, id int64, p ProfileUpdate) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET
			first_name  = COALESCE($2, first_name),
			last_name   = COALESCE($3, last_name),
			status_text = COALESCE($4, status_text),
			city        = COALESCE($5, city),
			about       = COALESCE($6, about),
			updated_at  = now()
		WHERE id = $1
		RETURNING `+userCols,
		id, p.FirstName, p.LastName, p.StatusText, p.City, p.About))
}

type PrivacyUpdate struct {
	OnlineVisibility  *string
	ProfileVisibility *string
	FriendsVisibility *string
	MessagePrivacy    *string
	Searchable        *bool
}

func (s *Store) UpdatePrivacy(ctx context.Context, id int64, p PrivacyUpdate) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		UPDATE users SET
			profile_visibility = COALESCE($2, profile_visibility),
			friends_visibility = COALESCE($3, friends_visibility),
			message_privacy    = COALESCE($4, message_privacy),
			searchable         = COALESCE($5, searchable),
			online_visibility  = COALESCE($6, online_visibility),
			updated_at         = now()
		WHERE id = $1
		RETURNING `+userCols,
		id, p.ProfileVisibility, p.FriendsVisibility, p.MessagePrivacy, p.Searchable, p.OnlineVisibility))
}

// SetAvatar ставит аватар только из собственных медиа пользователя (защита от IDOR).
func (s *Store) SetAvatar(ctx context.Context, userID int64, mediaID *string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET avatar_media_id = $2::uuid, updated_at = now()
		WHERE id = $1
		  AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM media WHERE id = $2::uuid AND owner_id = $1))`,
		userID, mediaID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBadMedia
	}
	return nil
}

func (s *Store) SetPassword(ctx context.Context, userID int64, hash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, hash)
	return err
}

// ---------- refresh-токены (сессии) ----------

type SessionMeta struct {
	UserAgent string
	IP        *netip.Addr
}

// RotateRefreshToken гасит старый токен и выдаёт новый в одной транзакции.
// Если пришёл уже отозванный токен, значит его украли и кто-то им уже
// воспользовался: отзываем все сессии пользователя (reuse detection).
func (s *Store) RotateRefreshToken(ctx context.Context, oldHash, newHash []byte, ttl time.Duration, meta SessionMeta) (userID int64, sid string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var (
		tokenID   int64
		expiresAt time.Time
		revokedAt *time.Time
		replaced  *time.Time
		started   time.Time
		bannedAt  *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT t.id, t.user_id, t.expires_at, t.revoked_at, t.replaced_at, t.session_started_at, t.session_id::text, u.banned_at
		FROM refresh_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		FOR UPDATE OF t`, oldHash).Scan(&tokenID, &userID, &expiresAt, &revokedAt, &replaced, &started, &sid, &bannedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrTokenInvalid
	}
	if err != nil {
		return 0, "", err
	}

	if revokedAt != nil && replaced == nil {
		return 0, "", ErrTokenInvalid // вышли или завершили сеанс — это не кража
	}
	if revokedAt != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE refresh_tokens SET revoked_at = now()
			WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return 0, "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, "", err
		}
		return userID, "", ErrTokenReused
	}
	if time.Now().After(expiresAt) || bannedAt != nil {
		return 0, "", ErrTokenInvalid
	}

	if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now(), replaced_at = now() WHERE id = $1`, tokenID); err != nil {
		return 0, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip, expires_at, session_started_at, session_id)
		VALUES ($1, $2, $3, $4, now() + $5::interval, $6, $7::uuid)`,
		userID, newHash, truncate(meta.UserAgent, 512), meta.IP, ttl, started, sid); err != nil {
		return 0, "", err
	}
	return userID, sid, tx.Commit(ctx)
}

func (s *Store) RevokeRefreshToken(ctx context.Context, hash []byte) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	return err
}

func (s *Store) RevokeAllSessions(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") // не режем многобайтный символ пополам
}
