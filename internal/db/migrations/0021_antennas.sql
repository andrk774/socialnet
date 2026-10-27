-- «Антенны» — свои ленты по ключевым словам: записи, где встречаются слова
-- (с учётом русских словоформ), без слов-исключений.

CREATE TABLE antennas (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
    keywords     TEXT[]      NOT NULL CHECK (cardinality(keywords) BETWEEN 1 AND 10),
    excludes     TEXT[]      NOT NULL DEFAULT '{}' CHECK (cardinality(excludes) <= 10),
    last_seen_id BIGINT      NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX antennas_user_idx ON antennas (user_id, id);

CREATE INDEX posts_text_fts_idx ON posts USING gin (to_tsvector('russian', text));
