ALTER TABLE voices ADD COLUMN media_type TEXT NOT NULL DEFAULT 'voice' CHECK (media_type IN ('voice','video'));
ALTER TABLE message_attachments DROP CONSTRAINT message_attachments_kind_check;
ALTER TABLE message_attachments ADD CONSTRAINT message_attachments_kind_check CHECK (kind IN ('photo','track','voice','document','video'));
