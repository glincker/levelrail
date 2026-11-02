-- Park snapshots the per-domain settings (WAF, TLS cert reference, auth gate,
-- redirect and so on) as JSON because releasing the claim deletes them.
-- Attach restores them; an empty value means a row parked before this existed.
ALTER TABLE parked_domains ADD COLUMN settings TEXT NOT NULL DEFAULT '';
