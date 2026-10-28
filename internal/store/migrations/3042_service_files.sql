-- Inline config files per service as a JSON array of {path, mode, secret,
-- content, hash}. A secret file keeps its content in the secrets store and
-- only its hash here. Existing rows start with none.
ALTER TABLE desired_services ADD COLUMN files TEXT NOT NULL DEFAULT '[]';
