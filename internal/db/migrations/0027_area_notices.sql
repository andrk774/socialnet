-- Предупреждения на территории: противопожарный режим, закрытые тропы
-- нацпарков, паводок. Ведут модераторы; показываются на маршрутах и
-- походах, чей участок пересекает прямоугольник предупреждения, в дни
-- действия.
CREATE TABLE area_notices (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind       TEXT        NOT NULL CHECK (kind IN ('fire', 'closure', 'flood', 'other')),
    title      TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body       TEXT        NOT NULL DEFAULT '' CHECK (char_length(body) <= 2000),
    source_url TEXT        NOT NULL DEFAULT '' CHECK (char_length(source_url) <= 500),
    min_lat    DOUBLE PRECISION NOT NULL CHECK (min_lat BETWEEN -90 AND 90),
    min_lon    DOUBLE PRECISION NOT NULL CHECK (min_lon BETWEEN -180 AND 180),
    max_lat    DOUBLE PRECISION NOT NULL CHECK (max_lat BETWEEN -90 AND 90),
    max_lon    DOUBLE PRECISION NOT NULL CHECK (max_lon BETWEEN -180 AND 180),
    starts_on  DATE        NOT NULL,
    ends_on    DATE,
    created_by BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (min_lat <= max_lat AND min_lon <= max_lon),
    CHECK (ends_on IS NULL OR ends_on >= starts_on)
);
CREATE INDEX area_notices_active_idx ON area_notices (ends_on);
