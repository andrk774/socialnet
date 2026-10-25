package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- OAuth-приложения ----------

type App struct {
	ID           int64     `json:"id"`
	OwnerID      int64     `json:"owner_id"`
	Name         string    `json:"name"`
	ClientID     string    `json:"client_id"`
	Confidential bool      `json:"confidential"`
	RedirectURIs []string  `json:"redirect_uris"`
	CreatedAt    time.Time `json:"created_at"`
	SecretHash   []byte    `json:"-"`
}

const appCols = `id, owner_id, name, client_id, secret_hash IS NOT NULL, redirect_uris, created_at, secret_hash`

func scanApp(row pgx.Row) (*App, error) {
	a := &App{}
	err := row.Scan(&a.ID, &a.OwnerID, &a.Name, &a.ClientID, &a.Confidential, &a.RedirectURIs, &a.CreatedAt, &a.SecretHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) CreateApp(ctx context.Context, owner int64, name, clientID string, secretHash []byte, uris []string) (*App, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM oauth_apps WHERE owner_id = $1`, owner).Scan(&n); err != nil {
		return nil, err
	}
	if n >= 20 {
		return nil, ErrLimit
	}
	return scanApp(s.pool.QueryRow(ctx, `
		INSERT INTO oauth_apps (owner_id, name, client_id, secret_hash, redirect_uris)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+appCols, owner, name, clientID, secretHash, uris))
}

func (s *Store) AppByClientID(ctx context.Context, clientID string) (*App, error) {
	return scanApp(s.pool.QueryRow(ctx, `SELECT `+appCols+` FROM oauth_apps WHERE client_id = $1`, clientID))
}

// OwnApp — приложение владельца; чужое выглядит как несуществующее.
func (s *Store) OwnApp(ctx context.Context, owner, id int64) (*App, error) {
	return scanApp(s.pool.QueryRow(ctx, `SELECT `+appCols+` FROM oauth_apps WHERE id = $1 AND owner_id = $2`, id, owner))
}

func (s *Store) MyApps(ctx context.Context, owner int64) ([]App, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+appCols+` FROM oauth_apps WHERE owner_id = $1 ORDER BY id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) UpdateApp(ctx context.Context, id int64, name *string, uris []string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE oauth_apps SET name = COALESCE($2, name), redirect_uris = COALESCE($3, redirect_uris) WHERE id = $1`,
		id, name, uris)
	return err
}

func (s *Store) SetAppSecret(ctx context.Context, id int64, hash []byte) error {
	_, err := s.pool.Exec(ctx, `UPDATE oauth_apps SET secret_hash = $2 WHERE id = $1`, id, hash)
	return err
}

func (s *Store) DeleteApp(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_apps WHERE id = $1`, id)
	return err
}

// ---------- коды авторизации ----------

func (s *Store) CreateAuthCode(ctx context.Context, hash []byte, appID, userID int64, scopes []string, redirect, challenge string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_codes (code_hash, app_id, user_id, scopes, redirect_uri, code_challenge, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)`, hash, appID, userID, scopes, redirect, challenge, ttl)
	return err
}

type AuthCode struct {
	ID          int64
	AppID       int64
	UserID      int64
	Scopes      []string
	RedirectURI string
	Challenge   string
}

// ErrCodeReused — код уже обменяли: по RFC 6749 §4.1.2 отзываем всё, что по нему выдано.
var ErrCodeReused = errors.New("authorization code reused")

// UseAuthCode атомарно гасит код. Повторное предъявление отзывает токены,
// выданные по этому коду: значит, код утёк.
func (s *Store) UseAuthCode(ctx context.Context, hash []byte) (*AuthCode, error) {
	c := &AuthCode{}
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_codes SET used_at = now()
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING id, app_id, user_id, scopes, redirect_uri, code_challenge`, hash).
		Scan(&c.ID, &c.AppID, &c.UserID, &c.Scopes, &c.RedirectURI, &c.Challenge)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = now()
		WHERE revoked_at IS NULL AND code_id = (SELECT id FROM oauth_codes WHERE code_hash = $1 AND used_at IS NOT NULL)`, hash)
	if err != nil {
		return nil, err
	}
	var used bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM oauth_codes WHERE code_hash = $1 AND used_at IS NOT NULL)`,
		hash).Scan(&used); err != nil {
		return nil, err
	}
	if used || tag.RowsAffected() > 0 {
		return nil, ErrCodeReused
	}
	return nil, ErrNotFound
}

// ---------- токены ----------

type NewAPIToken struct {
	Hash      []byte
	Kind      string // oauth | bot
	UserID    int64
	AppID     *int64
	CodeID    *int64
	Scopes    []string
	Name      string
	ExpiresAt *time.Time
}

func (s *Store) CreateAPIToken(ctx context.Context, t NewAPIToken) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO api_tokens (token_hash, kind, user_id, app_id, code_id, scopes, name, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		t.Hash, t.Kind, t.UserID, t.AppID, t.CodeID, t.Scopes, t.Name, t.ExpiresAt).Scan(&id)
	return id, err
}

type APITokenInfo struct {
	ID           int64
	Kind         string
	UserID       int64
	AppID        *int64
	Scopes       []string
	Banned       bool
	BotCommunity *int64
}

// APITokenAuth — проверка токена на каждый запрос. Отозванный,
// просроченный или токен удалённого приложения — ErrNotFound.
func (s *Store) APITokenAuth(ctx context.Context, hash []byte) (*APITokenInfo, error) {
	t := &APITokenInfo{}
	var bannedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE api_tokens t SET last_used_at = now()
		FROM users u
		WHERE t.token_hash = $1 AND u.id = t.user_id AND t.revoked_at IS NULL
		  AND (t.expires_at IS NULL OR t.expires_at > now())
		RETURNING t.id, t.kind, t.user_id, t.app_id, t.scopes, u.banned_at, u.bot_community_id`, hash).
		Scan(&t.ID, &t.Kind, &t.UserID, &t.AppID, &t.Scopes, &bannedAt, &t.BotCommunity)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	t.Banned = bannedAt != nil
	return t, err
}

// RevokeAPIToken (RFC 7009): отзывает, только если токен принадлежит приложению.
func (s *Store) RevokeAPIToken(ctx context.Context, hash []byte, appID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = now() WHERE token_hash = $1 AND app_id = $2 AND revoked_at IS NULL`, hash, appID)
	return err
}

// RevokeUserOAuthTokens — при смене и сбросе пароля, «выйти везде».
func (s *Store) RevokeUserOAuthTokens(ctx context.Context, user int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = now() WHERE user_id = $1 AND kind = 'oauth' AND revoked_at IS NULL`, user)
	return err
}

type AuthorizedApp struct {
	AppID      int64      `json:"app_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	GrantedAt  time.Time  `json:"granted_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (s *Store) AuthorizedApps(ctx context.Context, user int64) ([]AuthorizedApp, error) {
	const active = `t.user_id = $1 AND t.kind = 'oauth' AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > now())`
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.name,
		       ARRAY(SELECT DISTINCT sc FROM api_tokens t, unnest(t.scopes) sc WHERE `+active+` AND t.app_id = a.id ORDER BY 1),
		       min(t.created_at), max(t.last_used_at)
		FROM api_tokens t JOIN oauth_apps a ON a.id = t.app_id
		WHERE `+active+`
		GROUP BY a.id, a.name ORDER BY min(t.created_at)`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuthorizedApp{}
	for rows.Next() {
		var a AuthorizedApp
		if err := rows.Scan(&a.AppID, &a.Name, &a.Scopes, &a.GrantedAt, &a.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAppForUser(ctx context.Context, user, app int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = now() WHERE user_id = $1 AND app_id = $2 AND revoked_at IS NULL`, user, app)
	return tag.RowsAffected() > 0, err
}

// ---------- боты ----------

// BotPasswordHash — заведомо неверный хеш: войти паролем под ботом нельзя.
const BotPasswordHash = "!bot"

// EnsureBot создаёт служебного пользователя-бота для сообщества (если нет)
// и делает его редактором. Имя bot<id> зарезервировано при регистрации.
func (s *Store) EnsureBot(ctx context.Context, community int64, name, emailSuffix string) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE bot_community_id = $1`, community).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO users (email, username, password_hash, first_name, bot_community_id)
				VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				fmt.Sprintf("bot%d.%s@bots.invalid", community, emailSuffix), fmt.Sprintf("bot%d", community),
				BotPasswordHash, truncate(name, 64), community).Scan(&id)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO community_members (community_id, user_id, role) VALUES ($1, $2, 'editor')
			ON CONFLICT (community_id, user_id) DO UPDATE SET role = 'editor'`, community, id)
		return err
	})
	return id, err
}

func (s *Store) BotOf(ctx context.Context, community int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE bot_community_id = $1`, community).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// BotCommunity: если пользователь — бот, вернуть его сообщество.
func (s *Store) BotCommunity(ctx context.Context, user int64) (*int64, error) {
	var c *int64
	err := s.pool.QueryRow(ctx, `SELECT bot_community_id FROM users WHERE id = $1`, user).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

type BotToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (s *Store) BotTokens(ctx context.Context, bot int64) ([]BotToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, scopes, created_at, last_used_at FROM api_tokens
		WHERE user_id = $1 AND kind = 'bot' AND revoked_at IS NULL ORDER BY id`, bot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BotToken{}
	for rows.Next() {
		var t BotToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Scopes, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CountBotTokens(ctx context.Context, bot int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE user_id = $1 AND kind = 'bot' AND revoked_at IS NULL`, bot).Scan(&n)
	return n, err
}

func (s *Store) RevokeBotToken(ctx context.Context, bot, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND kind = 'bot' AND revoked_at IS NULL`, id, bot)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// UserWroteTo — писал ли from пользователю to в личном диалоге.
// Бот отвечает только тем, кто написал ему первым (как сообщество во ВКонтакте).
func (s *Store) UserWroteTo(ctx context.Context, from, to int64) (bool, error) {
	lo, hi := from, to
	if lo > hi {
		lo, hi = hi, lo
	}
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM conversations c JOIN messages m ON m.conversation_id = c.id
		               WHERE c.kind = 'direct' AND c.user_a = $1 AND c.user_b = $2 AND m.sender_id = $3)`,
		lo, hi, from).Scan(&ok)
	return ok, err
}

// ---------- Callback API ----------

type CallbackServer struct {
	CommunityID      int64     `json:"community_id"`
	URL              string    `json:"url"`
	Events           []string  `json:"events"`
	Status           string    `json:"status"`
	LastError        string    `json:"last_error"`
	UpdatedAt        time.Time `json:"updated_at"`
	SecretEnc        []byte    `json:"-"`
	ConfirmationCode string    `json:"-"`
}

func (s *Store) SetCallbackServer(ctx context.Context, cs CallbackServer) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO callback_servers (community_id, url, secret_enc, confirmation_code, events, status, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (community_id) DO UPDATE SET url = EXCLUDED.url, secret_enc = EXCLUDED.secret_enc,
			confirmation_code = EXCLUDED.confirmation_code, events = EXCLUDED.events,
			status = EXCLUDED.status, last_error = EXCLUDED.last_error, updated_at = now()`,
		cs.CommunityID, cs.URL, cs.SecretEnc, cs.ConfirmationCode, cs.Events, cs.Status, truncate(cs.LastError, 500))
	return err
}

func (s *Store) CallbackServer(ctx context.Context, community int64) (*CallbackServer, error) {
	cs := &CallbackServer{}
	err := s.pool.QueryRow(ctx, `
		SELECT community_id, url, events, status, last_error, updated_at, secret_enc, confirmation_code
		FROM callback_servers WHERE community_id = $1`, community).
		Scan(&cs.CommunityID, &cs.URL, &cs.Events, &cs.Status, &cs.LastError, &cs.UpdatedAt, &cs.SecretEnc, &cs.ConfirmationCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return cs, err
}

func (s *Store) DeleteCallbackServer(ctx context.Context, community int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM callback_servers WHERE community_id = $1`, community)
	return err
}

func (s *Store) SetCallbackStatus(ctx context.Context, community int64, status, lastErr string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE callback_servers SET status = $2, last_error = $3, updated_at = now() WHERE community_id = $1`,
		community, status, truncate(lastErr, 500))
	return err
}

// EnqueueCallback ставит событие в очередь, только если у сообщества есть
// подтверждённый сервер, подписанный на этот тип. Иначе — ничего не делает.
func (s *Store) EnqueueCallback(ctx context.Context, community int64, typ string, object any) error {
	payload, err := json.Marshal(object)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO callback_deliveries (community_id, type, payload)
		SELECT $1, $2, $3 FROM callback_servers
		WHERE community_id = $1 AND status <> 'unconfirmed' AND $2 = ANY (events)`, community, typ, payload)
	return err
}

type CallbackDelivery struct {
	ID          int64
	CommunityID int64
	EventID     string
	Type        string
	Payload     json.RawMessage
	Attempts    int
	URL         string
	SecretEnc   []byte
}

// ClaimCallback берёт следующую доставку вместе с адресом и секретом сервера.
func (s *Store) ClaimCallback(ctx context.Context) (*CallbackDelivery, error) {
	d := &CallbackDelivery{}
	err := s.pool.QueryRow(ctx, `
		WITH next AS (
			SELECT id FROM callback_deliveries
			WHERE status = 'queued' AND run_after <= now()
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		UPDATE callback_deliveries d SET status = 'sending', locked_at = now(), attempts = attempts + 1
		FROM next, callback_servers cs
		WHERE d.id = next.id AND cs.community_id = d.community_id
		RETURNING d.id, d.community_id, d.event_id::text, d.type, d.payload, d.attempts, cs.url, cs.secret_enc`).
		Scan(&d.ID, &d.CommunityID, &d.EventID, &d.Type, &d.Payload, &d.Attempts, &d.URL, &d.SecretEnc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

func (s *Store) FinishCallback(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE callback_deliveries SET status = 'done', locked_at = NULL, last_error = '' WHERE id = $1`, id)
	return err
}

// RetryCallback: после maxAttempts — failed; иначе повтор через backoff.
func (s *Store) RetryCallback(ctx context.Context, id int64, msg string, backoff time.Duration, maxAttempts int) (final bool, err error) {
	err = s.pool.QueryRow(ctx, `
		UPDATE callback_deliveries SET
			status = CASE WHEN attempts >= $4 THEN 'failed' ELSE 'queued' END,
			run_after = now() + $3::interval, locked_at = NULL, last_error = $2
		WHERE id = $1 RETURNING status = 'failed'`, id, truncate(msg, 500), backoff, maxAttempts).Scan(&final)
	return final, err
}

// RequeueStaleCallbacks — доставки, зависшие в «sending» (процесс упал).
func (s *Store) RequeueStaleCallbacks(ctx context.Context, after time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE callback_deliveries SET status = 'queued', locked_at = NULL
		WHERE status = 'sending' AND locked_at < now() - $1::interval`, after)
	return err
}

func (s *Store) CallbackDeliveries(ctx context.Context, community int64, limit int) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_id::text, type, status, attempts, last_error, created_at FROM callback_deliveries
		WHERE community_id = $1 ORDER BY id DESC LIMIT $2`, community, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ev, typ, st, le string
		var att int
		var at time.Time
		if err := rows.Scan(&ev, &typ, &st, &att, &le, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"event_id": ev, "type": typ, "status": st, "attempts": att, "last_error": le, "created_at": at})
	}
	return out, rows.Err()
}
