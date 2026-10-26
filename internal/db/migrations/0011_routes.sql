-- Маршруты из GPX: сводка, упрощённая геометрия и профиль высот.
-- Сам файл (очищенный GPX) лежит в хранилище медиа: routes/<id>/route.gpx.

CREATE TABLE routes (
    id           UUID        PRIMARY KEY,
    seq          BIGINT      GENERATED ALWAYS AS IDENTITY UNIQUE, -- курсор для списков
    owner_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    distance_m   INTEGER     NOT NULL CHECK (distance_m >= 0),
    gain_m       INTEGER     NOT NULL CHECK (gain_m >= 0),
    loss_m       INTEGER     NOT NULL CHECK (loss_m >= 0),
    min_ele      REAL,
    max_ele      REAL,
    duration_sec INTEGER     NOT NULL DEFAULT 0 CHECK (duration_sec >= 0),
    moving_sec   INTEGER     NOT NULL DEFAULT 0 CHECK (moving_sec >= 0),
    points       INTEGER     NOT NULL CHECK (points > 0),
    hidden_ends_m INTEGER    NOT NULL DEFAULT 0 CHECK (hidden_ends_m IN (0, 200, 500, 1000)),
    min_lat      DOUBLE PRECISION NOT NULL,
    min_lon      DOUBLE PRECISION NOT NULL,
    max_lat      DOUBLE PRECISION NOT NULL,
    max_lon      DOUBLE PRECISION NOT NULL,
    preview      JSONB       NOT NULL, -- до ~150 точек: схема в ленте
    detail       JSONB       NOT NULL, -- до ~3000 точек по сегментам: карта
    profile      JSONB       NOT NULL, -- профиль высот: [{d, e}]
    waypoints    JSONB       NOT NULL DEFAULT '[]',
    sha256       BYTEA       NOT NULL, -- исходного файла: для блок-листа после снятия
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    hidden_at    TIMESTAMPTZ
);
CREATE INDEX routes_owner_idx ON routes (owner_id, seq DESC) WHERE deleted_at IS NULL;

CREATE TABLE post_routes (
    post_id  BIGINT   NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    route_id UUID     NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    PRIMARY KEY (post_id, route_id)
);
CREATE INDEX post_routes_route_idx ON post_routes (route_id);

ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video', 'document', 'route'));
