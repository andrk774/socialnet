-- Поход как мероприятие: сложность, число мест, снаряжение, маршрут, место
-- сбора, заявки с одобрением организатора и лист ожидания.

ALTER TABLE events
    ADD COLUMN kind          TEXT     NOT NULL DEFAULT 'event' CHECK (kind IN ('event', 'hike')),
    ADD COLUMN difficulty    SMALLINT CHECK (difficulty BETWEEN 1 AND 5),
    ADD COLUMN capacity      INTEGER  CHECK (capacity BETWEEN 1 AND 500),
    ADD COLUMN gear          TEXT     NOT NULL DEFAULT '' CHECK (char_length(gear) <= 2000),
    ADD COLUMN route_id      UUID     REFERENCES routes(id) ON DELETE SET NULL,
    ADD COLUMN meeting_point TEXT     NOT NULL DEFAULT '' CHECK (char_length(meeting_point) <= 300),
    ADD COLUMN approval      BOOLEAN  NOT NULL DEFAULT false;
CREATE INDEX events_hikes_idx ON events (starts_at) WHERE kind = 'hike';
CREATE INDEX events_route_idx ON events (route_id) WHERE route_id IS NOT NULL;

-- pending — заявка ждёт организатора; waitlist — мест нет, очередь по updated_at.
ALTER TABLE event_attendees DROP CONSTRAINT event_attendees_status_check;
ALTER TABLE event_attendees
    ADD CONSTRAINT event_attendees_status_check CHECK (status IN ('going', 'maybe', 'pending', 'waitlist')),
    ADD COLUMN note TEXT NOT NULL DEFAULT '' CHECK (char_length(note) <= 500);
CREATE INDEX event_attendees_queue_idx ON event_attendees (event_id, status, updated_at);

-- уведомления о заявках и месте в походе ведут на мероприятие
ALTER TABLE notifications ADD COLUMN event_id BIGINT REFERENCES events(id) ON DELETE CASCADE;
