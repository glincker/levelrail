-- Owner of an API token: list/revoke are scoped to it, and a token whose
-- owner no longer exists is rejected on use. Empty means system-minted.
ALTER TABLE api_tokens ADD COLUMN owner_user_id TEXT NOT NULL DEFAULT '';
UPDATE api_tokens SET owner_user_id = COALESCE(
  (SELECT id FROM users WHERE abilities LIKE '%"root"%' ORDER BY created_at ASC LIMIT 1),
  (SELECT id FROM users ORDER BY created_at ASC LIMIT 1),
  ''
) WHERE owner_user_id = '' AND name NOT LIKE 'AI Assistant (internal)%';
CREATE INDEX IF NOT EXISTS idx_api_tokens_owner ON api_tokens (owner_user_id);
