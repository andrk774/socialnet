-- Вход по QR-коду: новое устройство показывает код, телефон, где вы уже
-- вошли, подтверждает. id виден в коде (по нему телефон находит запрос);
-- забрать сеанс может только устройство, у которого есть секрет опроса —
-- в базе лежит лишь его SHA-256. Запрос живёт 2 минуты и срабатывает один раз.
CREATE TABLE qr_logins (
    id          TEXT        PRIMARY KEY CHECK (char_length(id) BETWEEN 20 AND 64),
    poll_hash   BYTEA       NOT NULL,
    user_agent  TEXT        NOT NULL DEFAULT '' CHECK (char_length(user_agent) <= 500),
    ip          INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    approved_by BIGINT      REFERENCES users(id) ON DELETE CASCADE,
    approved_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    consumed_at TIMESTAMPTZ
);
CREATE INDEX qr_logins_expires_idx ON qr_logins (expires_at);
