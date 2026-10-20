-- SHA256 host key fingerprint the operator confirmed before credentials were sent.
-- Empty means the provision explicitly skipped host key verification.
ALTER TABLE ssh_node_provisions ADD COLUMN host_key_fingerprint TEXT NOT NULL DEFAULT '';
