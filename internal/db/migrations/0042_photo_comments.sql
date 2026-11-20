CREATE TABLE photo_comments (
 id bigserial PRIMARY KEY,
 photo_id uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
 author_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 text text NOT NULL CHECK(char_length(text) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX photo_comments_photo_id ON photo_comments(photo_id,id);
