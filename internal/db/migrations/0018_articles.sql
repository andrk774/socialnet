-- Статьи: длинные отчёты о походах с обложкой и безопасной разметкой
-- (исходный текст хранится, разбирается при выдаче — см. internal/markup).

CREATE TABLE articles (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title          TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 150),
    body           TEXT        NOT NULL DEFAULT '' CHECK (char_length(body) <= 100000),
    cover_media_id UUID        REFERENCES media(id) ON DELETE SET NULL,
    route_id       UUID        REFERENCES routes(id) ON DELETE SET NULL,
    status         TEXT        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ,
    hidden_at      TIMESTAMPTZ
);
CREATE INDEX articles_published_idx ON articles (id DESC) WHERE status = 'published' AND hidden_at IS NULL;
CREATE INDEX articles_author_idx ON articles (author_id, id DESC);

ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video', 'document', 'route',
                           'companion', 'route_point', 'article'));
