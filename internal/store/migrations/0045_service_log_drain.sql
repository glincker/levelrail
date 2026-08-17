-- External log-drain config for an app's service (Coolify parity gap,
-- docs-local/research/coolify-feature-comparison.md §8): forward this
-- service's container log stream to an HTTP endpoint or syslog target,
-- alongside the existing node-local store. Nullable JSON, same
-- storage_target_id convention (migrations/0030): SaveDesiredService
-- never writes this column, only UpdateServiceLogDrain does, so an
-- ordinary app edit can never silently attach or detach a drain.
ALTER TABLE desired_services ADD COLUMN log_drain TEXT;
