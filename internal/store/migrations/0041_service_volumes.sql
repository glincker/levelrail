-- Named-volume support for ordinary application services (previously
-- database-controller-only, internal/reconcile/database's own
-- dataVolumeName). JSON array, same storage shape env/resources/health
-- already use on this table, not a normal declarative desired-state
-- field excluded from SaveDesiredService's replace semantics.
ALTER TABLE desired_services ADD COLUMN volumes TEXT NOT NULL DEFAULT '[]';
