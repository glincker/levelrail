-- One immutable per-deployment URL: the retained service that answers a past
-- deploy attempt's hostname (internal/deployurls).
CREATE TABLE IF NOT EXISTS deploy_urls (
	service_name TEXT PRIMARY KEY,
	app_name     TEXT NOT NULL,
	attempt_id   TEXT NOT NULL,
	host         TEXT NOT NULL,
	created_at   TEXT NOT NULL,
	UNIQUE (app_name, attempt_id)
);

CREATE INDEX IF NOT EXISTS idx_deploy_urls_app ON deploy_urls (app_name);
