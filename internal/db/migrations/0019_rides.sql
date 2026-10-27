-- Подвоз к началу маршрута: водитель предлагает места, попутчики просятся,
-- водитель принимает. Деньги приложение не принимает и не считает.

CREATE TABLE rides (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    driver_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id    BIGINT      REFERENCES events(id) ON DELETE CASCADE,
    from_place  TEXT        NOT NULL CHECK (char_length(from_place) BETWEEN 1 AND 150),
    to_place    TEXT        NOT NULL CHECK (char_length(to_place) BETWEEN 1 AND 150),
    departs_at  TIMESTAMPTZ NOT NULL,
    seats       SMALLINT    NOT NULL CHECK (seats BETWEEN 1 AND 8),
    note        TEXT        NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX rides_departs_idx ON rides (departs_at);
CREATE INDEX rides_event_idx ON rides (event_id) WHERE event_id IS NOT NULL;

CREATE TABLE ride_requests (
    ride_id    BIGINT      NOT NULL REFERENCES rides(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted')),
    note       TEXT        NOT NULL DEFAULT '' CHECK (char_length(note) <= 300),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ride_id, user_id)
);
CREATE INDEX ride_requests_user_idx ON ride_requests (user_id);

ALTER TABLE notifications ADD COLUMN ride_id BIGINT REFERENCES rides(id) ON DELETE CASCADE;
