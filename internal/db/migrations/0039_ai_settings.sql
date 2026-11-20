CREATE TABLE ai_settings(id boolean PRIMARY KEY DEFAULT true CHECK(id), settings jsonb NOT NULL, key_enc bytea NOT NULL);
CREATE TABLE ai_usage(user_id bigint REFERENCES users(id) ON DELETE CASCADE, day date NOT NULL, kind text NOT NULL, calls integer NOT NULL, PRIMARY KEY(user_id,day,kind));
