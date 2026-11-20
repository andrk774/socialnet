CREATE TABLE photo_albums (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 100),
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE media ADD COLUMN album_id bigint REFERENCES photo_albums(id) ON DELETE SET NULL;
ALTER TABLE media ADD COLUMN archived boolean NOT NULL DEFAULT false;
CREATE INDEX media_owner_library ON media(owner_id, created_at DESC, id DESC);
CREATE INDEX photo_albums_owner ON photo_albums(owner_id);
