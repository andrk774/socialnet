-- API для разработчиков: OAuth-приложения, токены с правами (scopes),
-- боты сообществ и Callback API.

-- ---------- приложения ----------
CREATE TABLE oauth_apps (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    client_id     TEXT        NOT NULL UNIQUE,
    -- sha256 секрета; NULL — публичный клиент (SPA, мобильное приложение)
    secret_hash   BYTEA,
    redirect_uris TEXT[]      NOT NULL CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX oauth_apps_owner_idx ON oauth_apps (owner_id);

-- Код авторизации: одноразовый, живёт 10 минут, только с PKCE (S256).
CREATE TABLE oauth_codes (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code_hash      BYTEA       NOT NULL UNIQUE,
    app_id         BIGINT      NOT NULL REFERENCES oauth_apps(id) ON DELETE CASCADE,
    user_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes         TEXT[]      NOT NULL,
    redirect_uri   TEXT        NOT NULL,
    code_challenge TEXT        NOT NULL,
    expires_at     TIMESTAMPTZ NOT NULL,
    used_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------- боты сообществ ----------
-- Бот — служебный пользователь, привязанный к сообществу: ему можно
-- написать, он отвечает и публикует на стене сообщества. Войти паролем нельзя.
ALTER TABLE users ADD COLUMN bot_community_id BIGINT UNIQUE REFERENCES communities(id) ON DELETE CASCADE;

-- ---------- токены ----------
-- Хранится только sha256 токена. kind = oauth — от имени пользователя через
-- приложение; bot — токен бота сообщества.
CREATE TABLE api_tokens (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash   BYTEA       NOT NULL UNIQUE,
    kind         TEXT        NOT NULL CHECK (kind IN ('oauth', 'bot')),
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_id       BIGINT      REFERENCES oauth_apps(id) ON DELETE CASCADE,
    code_id      BIGINT      REFERENCES oauth_codes(id) ON DELETE SET NULL,
    scopes       TEXT[]      NOT NULL,
    name         TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,  -- NULL — бессрочный (scope offline или бот)
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    CHECK ((kind = 'oauth') = (app_id IS NOT NULL))
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id) WHERE revoked_at IS NULL;

-- ---------- Callback API ----------
CREATE TABLE callback_servers (
    community_id      BIGINT PRIMARY KEY REFERENCES communities(id) ON DELETE CASCADE,
    url               TEXT        NOT NULL CHECK (char_length(url) <= 500),
    secret_enc        BYTEA       NOT NULL,  -- секрет подписи, зашифрован AES-GCM
    confirmation_code TEXT        NOT NULL,
    events            TEXT[]      NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'unconfirmed' CHECK (status IN ('unconfirmed', 'active', 'failing')),
    last_error        TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Своя очередь доставки (а не jobs): её разбирает процесс API, у которого
-- есть выход в интернет. Изолированный воркер видео в сеть не ходит.
CREATE TABLE callback_deliveries (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    community_id BIGINT      NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    event_id     UUID        NOT NULL DEFAULT gen_random_uuid(),
    type         TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'done', 'failed')),
    attempts     INT         NOT NULL DEFAULT 0,
    run_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at    TIMESTAMPTZ,
    last_error   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX callback_deliveries_queue_idx ON callback_deliveries (run_after) WHERE status = 'queued';
