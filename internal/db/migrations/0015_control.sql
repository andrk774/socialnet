-- Контрольный срок похода: если группа не отметила возвращение, письмо
-- доверенным контактам участников. Контакты хранятся только на время
-- похода и удаляются после возвращения (или через 7 дней после срока).

ALTER TABLE events
    ADD COLUMN control_at       TIMESTAMPTZ,
    ADD COLUMN returned_at      TIMESTAMPTZ,
    ADD COLUMN reminder_sent_at TIMESTAMPTZ,
    ADD COLUMN alert_sent_at    TIMESTAMPTZ,
    ADD CONSTRAINT events_control_after_start CHECK (control_at IS NULL OR control_at > starts_at);
CREATE INDEX events_control_idx ON events (control_at)
    WHERE control_at IS NOT NULL AND returned_at IS NULL AND alert_sent_at IS NULL;

CREATE TABLE hike_contacts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id   BIGINT      NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    email      TEXT        NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, user_id, email)
);
CREATE INDEX hike_contacts_user_idx ON hike_contacts (user_id, created_at);
