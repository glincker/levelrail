-- Agent identity on API tokens, and on the audit entries made with them.
ALTER TABLE api_tokens ADD COLUMN agent_name TEXT NOT NULL DEFAULT '';
ALTER TABLE api_tokens ADD COLUMN agent_description TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN agent_name TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN agent_client TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_audit_log_agent ON audit_log (agent_name) WHERE agent_name != '';
