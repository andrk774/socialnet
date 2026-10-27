-- Отзывы: об организаторе — участник после похода, о маршруте каталога —
-- кто его прошёл. Один отзыв на поход или маршрут от человека, можно
-- исправить. Оценка 1–5 и необязательный текст.
CREATE TABLE reviews (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT        NOT NULL CHECK (kind IN ('organizer', 'route')),
    event_id     BIGINT      REFERENCES events(id) ON DELETE SET NULL,
    organizer_id BIGINT      REFERENCES users(id) ON DELETE CASCADE,
    route_id     UUID        REFERENCES routes(id) ON DELETE CASCADE,
    rating       SMALLINT    NOT NULL CHECK (rating BETWEEN 1 AND 5),
    text         TEXT        NOT NULL DEFAULT '' CHECK (char_length(text) <= 2000),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    hidden_at    TIMESTAMPTZ,
    CHECK ((kind = 'organizer' AND organizer_id IS NOT NULL AND route_id IS NULL) OR
           (kind = 'route' AND route_id IS NOT NULL AND organizer_id IS NULL AND event_id IS NULL))
);
-- отзыв об организаторе — один на поход; поход удалён — отзыв остаётся
CREATE UNIQUE INDEX reviews_event_author_idx ON reviews (event_id, author_id) WHERE kind = 'organizer' AND event_id IS NOT NULL;
CREATE UNIQUE INDEX reviews_route_author_idx ON reviews (route_id, author_id) WHERE kind = 'route';
CREATE INDEX reviews_organizer_idx ON reviews (organizer_id, id DESC) WHERE kind = 'organizer';
CREATE INDEX reviews_route_idx ON reviews (route_id, id DESC) WHERE kind = 'route';
CREATE INDEX reviews_author_idx ON reviews (author_id);

ALTER TABLE reports DROP CONSTRAINT reports_target_type_check;
ALTER TABLE reports ADD CONSTRAINT reports_target_type_check
    CHECK (target_type IN ('post', 'comment', 'user', 'community', 'track', 'video', 'document', 'route',
                           'companion', 'route_point', 'article', 'review'));
