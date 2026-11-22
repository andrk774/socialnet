ALTER TABLE messages ADD COLUMN edited_at timestamptz;
ALTER TABLE messages ADD COLUMN deleted_at timestamptz;
