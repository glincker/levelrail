-- One row per owned pre-library API token the upgrade import has decided on,
-- so the import stays idempotent and doctor can report what it did.
CREATE TABLE IF NOT EXISTS authengine_legacy_token_imports (
    legacy_id  TEXT PRIMARY KEY REFERENCES api_tokens (id) ON DELETE CASCADE,
    outcome    TEXT NOT NULL CHECK (outcome IN ('migrated', 'skipped')),
    reason     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
