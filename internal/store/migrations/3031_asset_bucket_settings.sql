-- Sidecar to storage_buckets rows with purpose 'assets' (ADR 044). Private and direct by default.
CREATE TABLE IF NOT EXISTS asset_bucket_settings (
	bucket_id         TEXT PRIMARY KEY REFERENCES storage_buckets (id) ON DELETE CASCADE,
	project_id        TEXT REFERENCES projects (id) ON DELETE SET NULL,
	visibility        TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
	serving_mode      TEXT NOT NULL DEFAULT 'direct' CHECK (serving_mode IN ('direct', 'proxied', 'cdn')),
	public_base_url   TEXT NOT NULL DEFAULT '',
	cdn_base_url      TEXT NOT NULL DEFAULT '',
	referrer_allow    TEXT NOT NULL DEFAULT '[]',
	refuse_no_referer INTEGER NOT NULL DEFAULT 0,
	updated_at        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_bucket_settings_project ON asset_bucket_settings (project_id);
