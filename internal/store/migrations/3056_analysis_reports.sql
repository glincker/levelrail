-- Deterministic incident and cost analysis reports (ADR 051, phase one, no model).
CREATE TABLE analysis_reports (
	id TEXT PRIMARY KEY,
	app TEXT NOT NULL,
	trigger_kind TEXT NOT NULL DEFAULT '',
	trigger_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	schema_version INTEGER NOT NULL DEFAULT 1,
	report TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL
);

CREATE INDEX analysis_reports_app_created ON analysis_reports (app, created_at);
