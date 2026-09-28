-- Agent certificate lifecycle (ADR 021): renewal with an overlap window,
-- revocation, and re-enrollment tokens bound to an existing node.
--
-- prev_cert_fingerprint stays accepted until prev_cert_valid_until so a
-- renewal whose response was lost cannot lock the node out.
-- cert_key_origin is 'server' for nodes enrolled before agents generated
-- their own keys, 'agent' after.
ALTER TABLE nodes ADD COLUMN cert_not_after TEXT;
ALTER TABLE nodes ADD COLUMN cert_serial TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN cert_renewed_at TEXT;
ALTER TABLE nodes ADD COLUMN cert_generation INTEGER NOT NULL DEFAULT 1;
ALTER TABLE nodes ADD COLUMN cert_key_origin TEXT NOT NULL DEFAULT 'server';
ALTER TABLE nodes ADD COLUMN prev_cert_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN prev_cert_valid_until TEXT;
ALTER TABLE nodes ADD COLUMN cert_revoked_at TEXT;

-- Existing nodes were issued 90 day certs at enrollment. The estimate is
-- replaced with the real NotAfter the next time the node connects.
UPDATE nodes
SET cert_not_after = strftime('%Y-%m-%dT%H:%M:%fZ', created_at, '+90 days')
WHERE cert_fingerprint != '';

-- purpose is 'enroll' (creates a new node) or 'reenroll' (issues a new
-- certificate to node_id, keeping its identity and history).
ALTER TABLE node_join_tokens ADD COLUMN purpose TEXT NOT NULL DEFAULT 'enroll';
ALTER TABLE node_join_tokens ADD COLUMN node_id TEXT REFERENCES nodes(id) ON DELETE CASCADE;
