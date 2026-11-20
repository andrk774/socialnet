ALTER TABLE comments ADD COLUMN media_ids uuid[] NOT NULL DEFAULT '{}',
 ADD COLUMN track_ids uuid[] NOT NULL DEFAULT '{}',
 ADD COLUMN video_ids uuid[] NOT NULL DEFAULT '{}',
 ADD COLUMN document_ids uuid[] NOT NULL DEFAULT '{}',
 ADD COLUMN sticker text NOT NULL DEFAULT '';
CREATE TABLE comment_reactions (
 comment_id bigint NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 reaction text NOT NULL CHECK (reaction IN ('like','love','haha','wow','sad','angry')),
 PRIMARY KEY(comment_id,user_id)
);
ALTER TABLE comments DROP CONSTRAINT comments_text_check;
ALTER TABLE comments ADD CONSTRAINT comments_text_check CHECK (char_length(text)<=2000 AND
 (text<>'' OR cardinality(media_ids)+cardinality(track_ids)+cardinality(video_ids)+cardinality(document_ids)>0 OR sticker<>''));
