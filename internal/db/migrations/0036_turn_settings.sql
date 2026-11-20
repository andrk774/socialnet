CREATE TABLE turn_settings (
 id boolean PRIMARY KEY DEFAULT true CHECK (id),
 enabled boolean NOT NULL,
 stun_url text NOT NULL,
 turn_url text NOT NULL,
 secret_enc bytea NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
