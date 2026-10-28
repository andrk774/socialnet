-- QR-код похода: сколько раз его отсканировали и сколько человек после
-- этого записались — по дням. Ни IP, ни пользователей не храним.
CREATE TABLE event_qr_stats (
    event_id BIGINT  NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    day      DATE    NOT NULL,
    scans    INTEGER NOT NULL DEFAULT 0,
    joins    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (event_id, day)
);
