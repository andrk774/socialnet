-- Этап 1: пользователи и сессии (refresh-токены).

-- citext: регистронезависимые строки, чтобы Ivan@Mail.ru и ivan@mail.ru
-- считались одним адресом на уровне уникального индекса.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email          CITEXT      NOT NULL,
    username       CITEXT      NOT NULL,
    password_hash  TEXT        NOT NULL,
    first_name     TEXT        NOT NULL DEFAULT '',
    last_name      TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_email_key    UNIQUE (email),
    CONSTRAINT users_username_key UNIQUE (username),
    CONSTRAINT users_email_len    CHECK (char_length(email) BETWEEN 3 AND 254),
    CONSTRAINT users_username_fmt CHECK (username ~ '^[A-Za-z0-9_]{3,32}$'),
    CONSTRAINT users_first_len    CHECK (char_length(first_name) <= 64),
    CONSTRAINT users_last_len     CHECK (char_length(last_name)  <= 64)
);

-- В базе хранится только SHA-256 от refresh-токена, а не сам токен:
-- утечка дампа БД не даёт войти в чужие аккаунты.
CREATE TABLE refresh_tokens (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  BYTEA       NOT NULL,
    user_agent  TEXT        NOT NULL DEFAULT '',
    ip          INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,

    CONSTRAINT refresh_tokens_hash_key UNIQUE (token_hash)
);

CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);
