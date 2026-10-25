-- Ветки комментариев, реакции, групповые чаты, вложения и голосовые,
-- истории, закрытые сообщества, мероприятия.

-- ---------- реакции вместо одного лайка ----------
-- Существующие лайки становятся реакцией 'like'; likes_count = все реакции.
ALTER TABLE post_likes
    ADD COLUMN reaction TEXT NOT NULL DEFAULT 'like'
        CHECK (reaction IN ('like', 'love', 'haha', 'wow', 'sad', 'angry'));

-- ---------- ветки комментариев ----------
-- Как во ВКонтакте: ответы живут под корневым комментарием (один уровень),
-- reply_to_id показывает, кому именно ответили.
ALTER TABLE comments
    ADD COLUMN parent_id     BIGINT REFERENCES comments(id) ON DELETE CASCADE,
    ADD COLUMN reply_to_id   BIGINT REFERENCES comments(id) ON DELETE SET NULL,
    ADD COLUMN replies_count INT NOT NULL DEFAULT 0;
CREATE INDEX comments_parent_idx ON comments (parent_id, id) WHERE parent_id IS NOT NULL;

-- ---------- групповые чаты ----------
-- conversation_state уже хранит строку на участника — она и становится
-- списком участников. Для личных диалогов user_a/user_b остаются как были.
ALTER TABLE conversations
    ADD COLUMN kind     TEXT NOT NULL DEFAULT 'direct' CHECK (kind IN ('direct', 'group')),
    ADD COLUMN title    TEXT NOT NULL DEFAULT '' CHECK (char_length(title) <= 100),
    ADD COLUMN owner_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ALTER COLUMN user_a DROP NOT NULL,
    ALTER COLUMN user_b DROP NOT NULL,
    ADD CONSTRAINT conversations_kind_users CHECK (
        (kind = 'direct' AND user_a IS NOT NULL AND user_b IS NOT NULL) OR
        (kind = 'group'  AND user_a IS NULL AND user_b IS NULL));

ALTER TABLE conversation_state
    ADD COLUMN role      TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    ADD COLUMN joined_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX conversation_state_user_idx ON conversation_state (user_id);
-- Доступ к чату теперь решает conversation_state: у каждого личного
-- диалога обе строки должны существовать.
INSERT INTO conversation_state (conversation_id, user_id)
SELECT id, user_a FROM conversations UNION ALL SELECT id, user_b FROM conversations
ON CONFLICT DO NOTHING;

-- Сообщение может состоять только из вложений; служебные сообщения
-- («Анна добавила Бориса») — kind = 'service'.
ALTER TABLE messages DROP CONSTRAINT messages_check;
ALTER TABLE messages
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'text' CHECK (kind IN ('text', 'service')),
    ADD CONSTRAINT messages_body_or_cipher CHECK (ciphertext IS NULL OR body = '');

-- Голосовые сообщения (MP3 моно). Доступ — только участникам чата.
CREATE TABLE voices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    duration_sec INT         NOT NULL CHECK (duration_sec > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE message_attachments (
    message_id BIGINT   NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    position   SMALLINT NOT NULL,
    kind       TEXT     NOT NULL CHECK (kind IN ('photo', 'track', 'voice')),
    media_id   UUID     REFERENCES media(id) ON DELETE CASCADE,
    track_id   UUID     REFERENCES tracks(id) ON DELETE CASCADE,
    voice_id   UUID     REFERENCES voices(id) ON DELETE CASCADE,
    PRIMARY KEY (message_id, position),
    CHECK (num_nonnulls(media_id, track_id, voice_id) = 1)
);
CREATE INDEX message_attachments_voice_idx ON message_attachments (voice_id) WHERE voice_id IS NOT NULL;

-- ---------- истории ----------
CREATE TABLE stories (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id   UUID        REFERENCES media(id) ON DELETE CASCADE,
    video_id   UUID        REFERENCES videos(id) ON DELETE CASCADE,
    caption    TEXT        NOT NULL DEFAULT '' CHECK (char_length(caption) <= 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
    CHECK (num_nonnulls(media_id, video_id) = 1)
);
CREATE INDEX stories_author_idx ON stories (author_id, expires_at);

CREATE TABLE story_views (
    story_id  BIGINT      NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    viewer_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (story_id, viewer_id)
);

-- ---------- закрытые сообщества ----------
ALTER TABLE communities
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'open' CHECK (visibility IN ('open', 'closed'));

CREATE TABLE community_join_requests (
    community_id BIGINT      NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (community_id, user_id)
);

-- ---------- мероприятия ----------
CREATE TABLE events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    creator_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    community_id BIGINT      REFERENCES communities(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description  TEXT        NOT NULL DEFAULT '' CHECK (char_length(description) <= 5000),
    place        TEXT        NOT NULL DEFAULT '' CHECK (char_length(place) <= 200),
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at IS NULL OR ends_at > starts_at)
);
CREATE INDEX events_starts_idx ON events (starts_at);

CREATE TABLE event_attendees (
    event_id   BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     TEXT        NOT NULL CHECK (status IN ('going', 'maybe')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id)
);
CREATE INDEX event_attendees_user_idx ON event_attendees (user_id);
