-- Opt-in apply, prune and deploy on stack sync, each off by default, plus the
-- commit a sync last handed to the apply path and the file:line of a failure.
ALTER TABLE stack_sync ADD COLUMN apply INTEGER NOT NULL DEFAULT 0;
ALTER TABLE stack_sync ADD COLUMN prune INTEGER NOT NULL DEFAULT 0;
ALTER TABLE stack_sync ADD COLUMN deploy INTEGER NOT NULL DEFAULT 0;
ALTER TABLE stack_sync ADD COLUMN applied_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE stack_sync ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE stack_sync ADD COLUMN detail TEXT NOT NULL DEFAULT '';
