-- Свои эмодзи сообщества: картинка (фото администратора) и код. Ими можно
-- реагировать на записи этого сообщества: реакция хранится как 'e:<id>'.

CREATE TABLE community_emoji (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    community_id BIGINT      NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    shortcode    TEXT        NOT NULL CHECK (shortcode ~ '^[a-z0-9_]{2,32}$'),
    media_id     UUID        NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    created_by   BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (community_id, shortcode)
);

ALTER TABLE post_likes DROP CONSTRAINT post_likes_reaction_check;
ALTER TABLE post_likes ADD CONSTRAINT post_likes_reaction_check
    CHECK (reaction IN ('like', 'love', 'haha', 'wow', 'sad', 'angry') OR reaction ~ '^e:[0-9]{1,18}$');
