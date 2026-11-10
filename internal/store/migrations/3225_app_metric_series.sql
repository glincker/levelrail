-- Where an app exposes its own metrics: port and path on its container, never a host.
CREATE TABLE app_metrics_targets (
	app        TEXT PRIMARY KEY,
	port       INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
	path       TEXT NOT NULL,
	last_error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

-- One row per distinct label set, so samples do not repeat label text.
CREATE TABLE app_metric_series (
	id     INTEGER PRIMARY KEY,
	app    TEXT NOT NULL,
	name   TEXT NOT NULL,
	labels TEXT NOT NULL DEFAULT '',
	UNIQUE (app, name, labels)
);

-- ts is unix milliseconds so range scans stay on the index.
CREATE TABLE app_metric_samples (
	series_id INTEGER NOT NULL,
	ts        INTEGER NOT NULL,
	value     REAL NOT NULL
);

CREATE INDEX app_metric_samples_series_ts ON app_metric_samples (series_id, ts);
CREATE INDEX app_metric_samples_ts ON app_metric_samples (ts);

-- A reused app name must never inherit another app's series.
CREATE TRIGGER desired_services_drop_app_metrics AFTER DELETE ON desired_services
BEGIN
	DELETE FROM app_metric_samples WHERE series_id IN (SELECT id FROM app_metric_series WHERE app = OLD.name);
	DELETE FROM app_metric_series WHERE app = OLD.name;
	DELETE FROM app_metrics_targets WHERE app = OLD.name;
END;
