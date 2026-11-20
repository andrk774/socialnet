CREATE TABLE video_views(video_id uuid REFERENCES videos(id) ON DELETE CASCADE,user_id bigint REFERENCES users(id) ON DELETE CASCADE,PRIMARY KEY(video_id,user_id));
CREATE TABLE profile_covers(user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,media_id uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE);
