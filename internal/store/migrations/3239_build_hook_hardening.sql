-- Build hook hardening: an optional source allowlist (comma-joined canonical
-- CIDRs, empty means any address), the time of the latest authenticated call,
-- and the latest refusal kept apart from the last build result.
ALTER TABLE build_hooks ADD COLUMN allowed_cidrs TEXT NOT NULL DEFAULT '';
ALTER TABLE build_hooks ADD COLUMN last_used_at TEXT;
ALTER TABLE build_hooks ADD COLUMN last_refusal TEXT NOT NULL DEFAULT '';
