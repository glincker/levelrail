-- CPU mode for models: compute kind, hard resource caps, the locally
-- managed weights file, download progress and the last benchmark result.
ALTER TABLE models ADD COLUMN compute TEXT NOT NULL DEFAULT 'gpu';
ALTER TABLE models ADD COLUMN cpu_milli INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN memory_mib INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN weights_repo TEXT NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN weights_file TEXT NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN weights_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN license_ack INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN download_done_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN download_total_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN benchmark_json TEXT NOT NULL DEFAULT '';
