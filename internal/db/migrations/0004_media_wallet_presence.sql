-- Онлайн-статус, настройки уведомлений, Web Push, кошелёк, музыка, видео.

-- ---------- онлайн-статус ----------
ALTER TABLE users
    ADD COLUMN last_seen_at      TIMESTAMPTZ,
    ADD COLUMN online_visibility TEXT NOT NULL DEFAULT 'everyone'
                                 CHECK (online_visibility IN ('everyone', 'friends', 'only_me')),
    ADD COLUMN last_digest_at    TIMESTAMPTZ;

-- ---------- настройки уведомлений ----------
CREATE TABLE notification_settings (
    user_id      BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    muted_types  TEXT[]  NOT NULL DEFAULT '{}',   -- такие уведомления не создаются
    email_digest BOOLEAN NOT NULL DEFAULT true,   -- письмо о непрочитанном
    push_enabled BOOLEAN NOT NULL DEFAULT true,
    quiet_start  SMALLINT CHECK (quiet_start BETWEEN 0 AND 1439), -- минуты от полуночи
    quiet_end    SMALLINT CHECK (quiet_end   BETWEEN 0 AND 1439),
    timezone     TEXT    NOT NULL DEFAULT 'UTC' CHECK (char_length(timezone) <= 64)
);

-- Подписки браузеров на Web Push.
CREATE TABLE push_subscriptions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT        NOT NULL UNIQUE CHECK (char_length(endpoint) <= 2048),
    p256dh     BYTEA       NOT NULL CHECK (octet_length(p256dh) = 65),
    auth       BYTEA       NOT NULL CHECK (octet_length(auth) = 16),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id);

-- ---------- внутренняя валюта ----------
-- Двойная запись: каждая операция переносит монеты с одного счёта на другой.
-- Системный счёт (kind = 'system') — источник наград и приёмник подарков;
-- только он может уходить в минус. Сумма всех балансов всегда равна нулю.
CREATE TABLE accounts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind       TEXT   NOT NULL CHECK (kind IN ('user', 'community', 'system')),
    owner_id   BIGINT,
    balance    BIGINT NOT NULL DEFAULT 0,
    UNIQUE (kind, owner_id),
    CHECK (kind = 'system' OR balance >= 0)   -- уйти в минус нельзя даже при гонке
);
INSERT INTO accounts (kind, owner_id) VALUES ('system', 0);

CREATE TABLE wallet_transactions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            TEXT   NOT NULL CHECK (kind IN ('transfer', 'gift', 'donation', 'reward', 'mint', 'purchase', 'payout')),
    from_account    BIGINT NOT NULL REFERENCES accounts(id),
    to_account      BIGINT NOT NULL REFERENCES accounts(id),
    amount          BIGINT NOT NULL CHECK (amount > 0),
    memo            TEXT   NOT NULL DEFAULT '' CHECK (char_length(memo) <= 200),
    initiator_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    idempotency_key TEXT   CHECK (char_length(idempotency_key) <= 64),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_account <> to_account),
    -- повтор запроса с тем же ключом не спишет деньги второй раз
    UNIQUE (initiator_id, idempotency_key)
);
CREATE INDEX wallet_tx_from_idx ON wallet_transactions (from_account, id DESC);
CREATE INDEX wallet_tx_to_idx   ON wallet_transactions (to_account, id DESC);

-- Ежедневный бонус: не больше одного в сутки.
CREATE TABLE daily_bonuses (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day     DATE   NOT NULL,
    PRIMARY KEY (user_id, day)
);

CREATE TABLE gift_catalog (
    id    INT PRIMARY KEY,
    name  TEXT   NOT NULL,
    price BIGINT NOT NULL CHECK (price > 0)
);
INSERT INTO gift_catalog (id, name, price) VALUES
    (1, 'Сердце', 10), (2, 'Цветы', 25), (3, 'Торт', 50), (4, 'Медведь', 100), (5, 'Корона', 500);

CREATE TABLE gifts (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    gift_id        INT    NOT NULL REFERENCES gift_catalog(id),
    sender_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message        TEXT   NOT NULL DEFAULT '' CHECK (char_length(message) <= 200),
    anonymous      BOOLEAN NOT NULL DEFAULT false,
    transaction_id BIGINT NOT NULL REFERENCES wallet_transactions(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX gifts_recipient_idx ON gifts (recipient_id, id DESC);

-- ---------- музыка и видео ----------
-- Хэши снятых по жалобе правообладателя файлов: повторно их не загрузить.
CREATE TABLE content_blocklist (
    sha256     BYTEA PRIMARY KEY CHECK (octet_length(sha256) = 32),
    kind       TEXT  NOT NULL,
    reason     TEXT  NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tracks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL CHECK (char_length(title)  BETWEEN 1 AND 200),
    artist       TEXT        NOT NULL CHECK (char_length(artist) BETWEEN 1 AND 200),
    duration_sec INT         NOT NULL CHECK (duration_sec > 0),
    size_bytes   BIGINT      NOT NULL,
    source_sha256 BYTEA      NOT NULL,
    removed_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tracks_uploader_idx ON tracks (uploader_id);
CREATE INDEX tracks_search_idx ON tracks USING gin ((lower(artist || ' ' || title)) gin_trgm_ops);

CREATE TABLE user_tracks (
    user_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id UUID        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    seq      BIGINT GENERATED ALWAYS AS IDENTITY,
    PRIMARY KEY (user_id, track_id)
);
CREATE INDEX user_tracks_seq_idx ON user_tracks (user_id, seq DESC);

CREATE TABLE playlists (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX playlists_owner_idx ON playlists (owner_id);

CREATE TABLE playlist_tracks (
    playlist_id BIGINT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    UUID   NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INT    NOT NULL,
    PRIMARY KEY (playlist_id, track_id)
);

CREATE TABLE post_tracks (
    post_id  BIGINT   NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    track_id UUID     NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    PRIMARY KEY (post_id, track_id)
);

CREATE TABLE videos (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title         TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description   TEXT        NOT NULL DEFAULT '' CHECK (char_length(description) <= 5000),
    status        TEXT        NOT NULL DEFAULT 'processing'
                              CHECK (status IN ('processing', 'ready', 'failed', 'removed')),
    duration_sec  INT,
    width         INT,
    height        INT,
    renditions    TEXT[]      NOT NULL DEFAULT '{}',
    source_sha256 BYTEA       NOT NULL,
    error         TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_at      TIMESTAMPTZ
);
CREATE INDEX videos_owner_idx ON videos (owner_id, created_at DESC);

CREATE TABLE post_videos (
    post_id  BIGINT   NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    video_id UUID     NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    PRIMARY KEY (post_id, video_id)
);

-- Очередь фоновых задач в PostgreSQL: воркеры берут задачи через
-- FOR UPDATE SKIP LOCKED, поэтому их можно запускать несколько.
CREATE TABLE jobs (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind       TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts   INT         NOT NULL DEFAULT 0,
    run_after  TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at  TIMESTAMPTZ,
    error      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX jobs_queue_idx ON jobs (run_after) WHERE status = 'queued';

-- жаловаться можно и на треки с видео
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video'));
-- id треков и видео — UUID, поэтому храним цель жалобы ещё и строкой
ALTER TABLE reports ADD COLUMN target_uuid UUID;
ALTER TABLE reports DROP CONSTRAINT reports_reporter_id_target_type_target_id_key;
CREATE UNIQUE INDEX reports_unique_target ON reports (reporter_id, target_type, target_id, COALESCE(target_uuid, '00000000-0000-0000-0000-000000000000'));
ALTER TABLE moderation_log ADD COLUMN target_uuid UUID;
