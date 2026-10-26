-- «Ищу попутчиков»: объявление с районом, датами, сложностью и числом мест.
-- Отклик уходит автору личным сообщением.

CREATE TABLE companion_ads (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    region      TEXT        NOT NULL CHECK (char_length(region) BETWEEN 1 AND 80),
    title       TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    body        TEXT        NOT NULL DEFAULT '' CHECK (char_length(body) <= 2000),
    date_from   DATE        NOT NULL,
    date_to     DATE        NOT NULL,
    difficulty  SMALLINT    CHECK (difficulty BETWEEN 1 AND 5),
    seats       SMALLINT    NOT NULL DEFAULT 1 CHECK (seats BETWEEN 1 AND 20),
    route_id    UUID        REFERENCES routes(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at   TIMESTAMPTZ,
    hidden_at   TIMESTAMPTZ,
    CHECK (date_to >= date_from AND date_to - date_from <= 60)
);
CREATE INDEX companion_ads_open_idx ON companion_ads (date_from, id) WHERE closed_at IS NULL AND hidden_at IS NULL;
CREATE INDEX companion_ads_author_idx ON companion_ads (author_id, id DESC);

ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video', 'document', 'route', 'companion'));
