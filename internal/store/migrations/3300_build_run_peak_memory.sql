-- Build cgroup memory high water mark, so an OOM build can suggest a cap from what it used.
ALTER TABLE build_runs ADD COLUMN peak_memory_mb INTEGER NOT NULL DEFAULT 0;
