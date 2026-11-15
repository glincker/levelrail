-- Commit statuses and pipeline events Levelrail posts to a forge, kept per app and commit
-- for the delivery timeline. Additive: older binaries never read it.
CREATE TABLE forge_status_reports (
  id          TEXT PRIMARY KEY,
  app_name    TEXT NOT NULL,
  provider    TEXT NOT NULL DEFAULT '',
  sha         TEXT NOT NULL,
  context     TEXT NOT NULL DEFAULT '',
  state       TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  target_url  TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT '',
  error       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);
CREATE INDEX forge_status_reports_app_time ON forge_status_reports (app_name, created_at DESC);
CREATE INDEX forge_status_reports_created ON forge_status_reports (created_at);
