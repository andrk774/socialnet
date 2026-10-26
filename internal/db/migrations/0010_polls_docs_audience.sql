-- Опросы, закладки, аудитория поста, отложенные публикации, документы.

-- ---------- аудитория поста ----------
-- everyone — все, кому виден профиль; friends — только друзья;
-- only_me — только автор; list — выбранные друзья (post_audience).
-- Для постов сообществ — всегда everyone (видимость решает сообщество).
ALTER TABLE posts ADD COLUMN audience TEXT NOT NULL DEFAULT 'everyone'
    CHECK (audience IN ('everyone', 'friends', 'only_me', 'list'));
ALTER TABLE posts ADD CONSTRAINT posts_community_audience CHECK (community_id IS NULL OR audience = 'everyone');

CREATE TABLE post_audience (
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, user_id)
);
CREATE INDEX post_audience_user_idx ON post_audience (user_id);

-- ---------- опросы ----------
CREATE TABLE polls (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id      BIGINT      NOT NULL UNIQUE REFERENCES posts(id) ON DELETE CASCADE,
    question     TEXT        NOT NULL CHECK (char_length(question) BETWEEN 1 AND 255),
    multiple     BOOLEAN     NOT NULL DEFAULT false,
    anonymous    BOOLEAN     NOT NULL DEFAULT false,
    closes_at    TIMESTAMPTZ,
    voters_count INTEGER     NOT NULL DEFAULT 0 CHECK (voters_count >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE poll_options (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    poll_id     BIGINT   NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    position    SMALLINT NOT NULL,
    text        TEXT     NOT NULL CHECK (char_length(text) BETWEEN 1 AND 100),
    votes_count INTEGER  NOT NULL DEFAULT 0 CHECK (votes_count >= 0),
    UNIQUE (poll_id, position)
);

CREATE TABLE poll_votes (
    poll_id    BIGINT      NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id  BIGINT      NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (poll_id, user_id, option_id)
);
CREATE INDEX poll_votes_option_idx ON poll_votes (option_id, user_id);

-- ---------- закладки ----------
CREATE TABLE bookmarks (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL CHECK (kind IN ('post', 'video', 'track')),
    target     TEXT        NOT NULL CHECK (char_length(target) <= 64), -- id поста или UUID
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, kind, target)
);
CREATE INDEX bookmarks_list_idx ON bookmarks (user_id, kind, id DESC);

-- ---------- отложенные публикации ----------
-- payload — проверенный запрос на создание поста. При публикации права и
-- вложения проверяются заново: за это время автора могли разжаловать.
CREATE TABLE scheduled_posts (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    community_id BIGINT      REFERENCES communities(id) ON DELETE CASCADE,
    publish_at   TIMESTAMPTZ NOT NULL,
    payload      JSONB       NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed', 'canceled')),
    post_id      BIGINT      REFERENCES posts(id) ON DELETE SET NULL,
    error        TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX scheduled_posts_due_idx ON scheduled_posts (publish_at) WHERE status = 'pending';
CREATE INDEX scheduled_posts_author_idx ON scheduled_posts (author_id, publish_at) WHERE status = 'pending';
CREATE INDEX scheduled_posts_community_idx ON scheduled_posts (community_id, publish_at) WHERE status = 'pending';

-- ---------- документы ----------
CREATE TABLE documents (
    id         UUID        PRIMARY KEY,
    seq        BIGINT      GENERATED ALWAYS AS IDENTITY UNIQUE, -- курсор для списков
    owner_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title     TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    ext        TEXT        NOT NULL CHECK (ext ~ '^[a-z0-9]{1,5}$'),
    size_bytes BIGINT      NOT NULL CHECK (size_bytes > 0),
    sha256     BYTEA       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    hidden_at  TIMESTAMPTZ
);
CREATE INDEX documents_owner_idx ON documents (owner_id, seq DESC) WHERE deleted_at IS NULL;

CREATE TABLE post_documents (
    post_id     BIGINT   NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    document_id UUID     NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    position    SMALLINT NOT NULL,
    PRIMARY KEY (post_id, document_id)
);
CREATE INDEX post_documents_doc_idx ON post_documents (document_id);

ALTER TABLE message_attachments ADD COLUMN document_id UUID REFERENCES documents(id) ON DELETE CASCADE;
ALTER TABLE message_attachments DROP CONSTRAINT message_attachments_kind_check;
ALTER TABLE message_attachments ADD CONSTRAINT message_attachments_kind_check
    CHECK (kind IN ('photo', 'track', 'voice', 'document'));
ALTER TABLE message_attachments DROP CONSTRAINT message_attachments_check;
ALTER TABLE message_attachments ADD CONSTRAINT message_attachments_one_target
    CHECK (num_nonnulls(media_id, track_id, voice_id, document_id) = 1);
CREATE INDEX message_attachments_doc_idx ON message_attachments (document_id) WHERE document_id IS NOT NULL;

-- Жалобы и скрытие модератором распространяются и на документы.
ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video', 'document'));
