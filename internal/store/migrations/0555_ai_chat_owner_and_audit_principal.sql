-- Chat sessions belong to the principal that created them ("user:<id>" or
-- "token:<id>"); rows from before this migration have no owner and are
-- visible to nobody. on_behalf_of names the human an AI turn token acted for.
ALTER TABLE ai_chat_sessions ADD COLUMN owner_principal TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_ai_chat_sessions_owner ON ai_chat_sessions(owner_principal, updated_at);
ALTER TABLE audit_log ADD COLUMN on_behalf_of TEXT NOT NULL DEFAULT '';
