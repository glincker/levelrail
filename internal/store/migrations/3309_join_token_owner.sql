-- User who minted a personal builder join token, recorded on the node it enrols.
ALTER TABLE node_join_tokens ADD COLUMN owner_id TEXT NOT NULL DEFAULT '';
