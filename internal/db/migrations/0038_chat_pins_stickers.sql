CREATE TABLE conversation_pins (
 conversation_id bigint PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
 message_id bigint NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
 pinned_by bigint NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE custom_stickers (
 id uuid PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
 owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 title text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX custom_stickers_owner ON custom_stickers(owner_id,created_at DESC);
