CREATE TABLE smtp_settings (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),
 host text NOT NULL, port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
 username text NOT NULL, sender text NOT NULL, tls_mode text NOT NULL,
 password_enc bytea NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
