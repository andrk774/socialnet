CREATE TABLE nearby_presence (
 user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 lat double precision NOT NULL CHECK(lat BETWEEN -90 AND 90),
 lon double precision NOT NULL CHECK(lon BETWEEN -180 AND 180),
 expires_at timestamptz NOT NULL
);
CREATE INDEX nearby_presence_expiry ON nearby_presence(expires_at);
