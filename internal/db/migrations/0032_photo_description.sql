ALTER TABLE media ADD COLUMN description text NOT NULL DEFAULT '' CHECK (char_length(description)<=2000);
