-- Structured JSON detail for lifecycle audit rows (maintenance start, duration, end reason).
ALTER TABLE audit_log ADD COLUMN detail TEXT NOT NULL DEFAULT '';
