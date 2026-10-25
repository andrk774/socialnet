-- Этапы 2+: профили, друзья, приватность, посты, сообщения, медиа,
-- безопасность аккаунта, сообщества, модерация.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ---------- медиа ----------
CREATE TABLE media (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mime        TEXT        NOT NULL CHECK (mime IN ('image/jpeg', 'image/png')),
    width       INT         NOT NULL CHECK (width  > 0),
    height      INT         NOT NULL CHECK (height > 0),
    size_bytes  BIGINT      NOT NULL CHECK (size_bytes > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX media_owner_idx ON media (owner_id);

-- ---------- профиль, роли, приватность, 2FA ----------
ALTER TABLE users
    ADD COLUMN status_text        TEXT NOT NULL DEFAULT '' CHECK (char_length(status_text) <= 140),
    ADD COLUMN city               TEXT NOT NULL DEFAULT '' CHECK (char_length(city) <= 64),
    ADD COLUMN about              TEXT NOT NULL DEFAULT '' CHECK (char_length(about) <= 1000),
    ADD COLUMN avatar_media_id    UUID REFERENCES media(id) ON DELETE SET NULL,
    ADD COLUMN email_verified_at  TIMESTAMPTZ,
    ADD COLUMN role               TEXT NOT NULL DEFAULT 'user'
                                  CHECK (role IN ('user', 'moderator', 'admin')),
    ADD COLUMN banned_at          TIMESTAMPTZ,
    ADD COLUMN ban_reason         TEXT NOT NULL DEFAULT '',
    ADD COLUMN profile_visibility TEXT NOT NULL DEFAULT 'everyone'
                                  CHECK (profile_visibility IN ('everyone', 'friends', 'only_me')),
    ADD COLUMN friends_visibility TEXT NOT NULL DEFAULT 'everyone'
                                  CHECK (friends_visibility IN ('everyone', 'friends', 'only_me')),
    ADD COLUMN message_privacy    TEXT NOT NULL DEFAULT 'everyone'
                                  CHECK (message_privacy IN ('everyone', 'friends', 'nobody')),
    ADD COLUMN searchable         BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN totp_secret_enc    BYTEA,          -- секрет TOTP, зашифрованный AES-GCM
    ADD COLUMN totp_enabled_at    TIMESTAMPTZ,    -- NULL при заданном секрете = настройка не завершена
    ADD COLUMN totp_last_step     BIGINT NOT NULL DEFAULT 0;  -- защита от повторного кода

CREATE INDEX users_search_idx ON users
    USING gin ((lower(username::text || ' ' || first_name || ' ' || last_name)) gin_trgm_ops);

-- Сессия = цепочка ротаций refresh-токенов; помним, когда она началась.
ALTER TABLE refresh_tokens
    ADD COLUMN session_started_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- ---------- друзья как во ВКонтакте ----------
-- Заявка в друзья = подписка. Взаимная подписка = дружба.
CREATE TABLE follows (
    follower_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
);
CREATE INDEX follows_followee_idx ON follows (followee_id);

CREATE TABLE blocks (
    blocker_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);
CREATE INDEX blocks_blocked_idx ON blocks (blocked_id);

-- ---------- сообщества ----------
CREATE TABLE communities (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug        CITEXT      NOT NULL UNIQUE CHECK (slug ~ '^[A-Za-z0-9_]{3,32}$'),
    name        TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    description TEXT        NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    owner_id    BIGINT      NOT NULL REFERENCES users(id),
    hidden_at   TIMESTAMPTZ,  -- скрыто модератором
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX communities_search_idx ON communities
    USING gin ((lower(slug::text || ' ' || name)) gin_trgm_ops);

CREATE TABLE community_members (
    community_id BIGINT      NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT        NOT NULL DEFAULT 'member'
                             CHECK (role IN ('owner', 'admin', 'editor', 'member')),
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (community_id, user_id)
);
CREATE INDEX community_members_user_idx ON community_members (user_id);

-- ---------- посты ----------
CREATE TABLE posts (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    community_id   BIGINT      REFERENCES communities(id) ON DELETE CASCADE,
    text           TEXT        NOT NULL DEFAULT '' CHECK (char_length(text) <= 10000),
    likes_count    INT         NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    comments_count INT         NOT NULL DEFAULT 0 CHECK (comments_count >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at      TIMESTAMPTZ,
    deleted_at     TIMESTAMPTZ,
    hidden_at      TIMESTAMPTZ   -- скрыт модератором
);
CREATE INDEX posts_author_idx    ON posts (author_id, id DESC) WHERE community_id IS NULL;
CREATE INDEX posts_community_idx ON posts (community_id, id DESC) WHERE community_id IS NOT NULL;

CREATE TABLE post_media (
    post_id  BIGINT   NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    media_id UUID     NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    PRIMARY KEY (post_id, media_id)
);

CREATE TABLE post_likes (
    post_id    BIGINT      NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);

CREATE TABLE comments (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id    BIGINT      NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text       TEXT        NOT NULL CHECK (char_length(text) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    hidden_at  TIMESTAMPTZ
);
CREATE INDEX comments_post_idx ON comments (post_id, id);

CREATE TABLE hashtags (
    id  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tag CITEXT NOT NULL UNIQUE CHECK (char_length(tag) BETWEEN 1 AND 64)
);
CREATE TABLE post_hashtags (
    post_id    BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    hashtag_id BIGINT NOT NULL REFERENCES hashtags(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, hashtag_id)
);
CREATE INDEX post_hashtags_tag_idx ON post_hashtags (hashtag_id, post_id DESC);

-- ---------- личные сообщения ----------
CREATE TABLE conversations (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_a     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_b     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_message_id BIGINT,
    UNIQUE (user_a, user_b),
    CHECK (user_a < user_b)
);
CREATE INDEX conversations_b_idx ON conversations (user_b);

CREATE TABLE messages (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT      NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body            TEXT        NOT NULL DEFAULT '' CHECK (char_length(body) <= 4000),
    -- для сквозного шифрования: сервер хранит только шифротекст
    ciphertext      BYTEA       CHECK (octet_length(ciphertext) <= 16384),
    nonce           BYTEA       CHECK (octet_length(nonce) <= 64),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((ciphertext IS NULL AND body <> '') OR (ciphertext IS NOT NULL AND body = ''))
);
CREATE INDEX messages_conv_idx ON messages (conversation_id, id DESC);

CREATE TABLE conversation_state (
    conversation_id   BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_delivered_id BIGINT NOT NULL DEFAULT 0,
    last_read_id      BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (conversation_id, user_id)
);

-- Публичный ключ X25519 для сквозного шифрования (32 байта).
CREATE TABLE user_keys (
    user_id      BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    identity_key BYTEA       NOT NULL CHECK (octet_length(identity_key) = 32),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------- уведомления ----------
CREATE TABLE notifications (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,
    actor_id   BIGINT      REFERENCES users(id) ON DELETE CASCADE,
    post_id    BIGINT      REFERENCES posts(id) ON DELETE CASCADE,
    comment_id BIGINT      REFERENCES comments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ
);
CREATE INDEX notifications_user_idx ON notifications (user_id, id DESC);

-- ---------- безопасность аккаунта ----------
-- Одноразовые токены из писем. В базе только SHA-256.
CREATE TABLE auth_tokens (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    TEXT        NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    token_hash BYTEA       NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auth_tokens_user_idx ON auth_tokens (user_id, purpose);

CREATE TABLE backup_codes (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash BYTEA       NOT NULL,
    used_at   TIMESTAMPTZ
);
CREATE INDEX backup_codes_user_idx ON backup_codes (user_id);

CREATE TABLE security_events (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,
    ip         INET,
    user_agent TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX security_events_user_idx ON security_events (user_id, id DESC);

-- ---------- модерация ----------
CREATE TABLE reports (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reporter_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type TEXT        NOT NULL CHECK (target_type IN ('post', 'comment', 'user', 'community')),
    target_id   BIGINT      NOT NULL,
    reason      TEXT        NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 1000),
    status      TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'dismissed')),
    handled_by  BIGINT      REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    handled_at  TIMESTAMPTZ,
    UNIQUE (reporter_id, target_type, target_id)  -- одна жалоба от человека на объект
);
CREATE INDEX reports_open_idx ON reports (id) WHERE status = 'open';

CREATE TABLE moderation_log (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    moderator_id BIGINT      NOT NULL REFERENCES users(id),
    action       TEXT        NOT NULL,
    target_type  TEXT        NOT NULL,
    target_id    BIGINT      NOT NULL,
    report_id    BIGINT      REFERENCES reports(id),
    note         TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Сообщения об уязвимостях от внешних исследователей.
CREATE TABLE vuln_reports (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    contact     TEXT        NOT NULL CHECK (char_length(contact) BETWEEN 3 AND 254),
    title       TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description TEXT        NOT NULL CHECK (char_length(description) BETWEEN 1 AND 20000),
    ip          INET,
    status      TEXT        NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'triaged', 'fixed', 'rejected')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
