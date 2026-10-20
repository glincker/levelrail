-- Per-app opt-out for shell/exec access: POST /apps/{name}/exec and GET
-- /apps/{name}/terminal both check this flag before attempting to reach
-- the container, in addition to (not instead of) the existing
-- AbilityRoot IAM check on both routes. Default true (exec is available
-- today with no equivalent gate), unlike auto_rollback_on_crashloop's
-- default false: this preserves existing behavior for every app that
-- never touches the new setting, and an operator disables it explicitly
-- per app to lock down shell access even for an otherwise-root token.
ALTER TABLE desired_services ADD COLUMN exec_enabled INTEGER NOT NULL DEFAULT 1;
