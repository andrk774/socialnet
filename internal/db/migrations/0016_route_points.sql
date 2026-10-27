-- Точки на маршруте от тех, кто по нему ходил: родник, стоянка, брод,
-- опасное место, вид. Фото, заметка и дата последней проверки на месте.

CREATE TABLE route_points (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    route_id    UUID        NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    author_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT        NOT NULL CHECK (kind IN ('spring', 'camp', 'ford', 'danger', 'view', 'other')),
    lat         DOUBLE PRECISION NOT NULL CHECK (lat BETWEEN -90 AND 90),
    lon         DOUBLE PRECISION NOT NULL CHECK (lon BETWEEN -180 AND 180),
    title       TEXT        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 80),
    note        TEXT        NOT NULL DEFAULT '' CHECK (char_length(note) <= 1000),
    photo_id    UUID        REFERENCES media(id) ON DELETE SET NULL,
    verified_on DATE        NOT NULL DEFAULT current_date, -- последний раз видели на месте
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    hidden_at   TIMESTAMPTZ
);
CREATE INDEX route_points_route_idx ON route_points (route_id, id);
CREATE INDEX route_points_author_idx ON route_points (author_id, created_at);

-- «Актуально»: кто и когда подтвердил, что точка на месте.
CREATE TABLE route_point_checks (
    point_id   BIGINT      NOT NULL REFERENCES route_points(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checked_on DATE        NOT NULL DEFAULT current_date,
    PRIMARY KEY (point_id, user_id)
);

ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video', 'document', 'route', 'companion', 'route_point'));
