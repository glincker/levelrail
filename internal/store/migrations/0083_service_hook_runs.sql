-- Latest pre/post-deploy hook execution outcome per service
-- (internal/reconcile/application's own HookRunRecorder), one row per
-- (service_name, hook_type): the reconciler upserts this on every hook
-- run, success or failure, so GET /api/v1/apps/{name}/hook-runs can show
-- an operator what actually happened without them needing to have been
-- watching a live log at the moment it ran. Deliberately not a full
-- history log: the reconciler is level-triggered and re-runs a hook on
-- every fresh container it creates for a service (application.Controller's
-- own doc comment), so only the most recent outcome per hook type is a
-- meaningful, bounded thing to keep.
CREATE TABLE service_hook_runs (
	service_name TEXT NOT NULL,
	hook_type    TEXT NOT NULL,
	command      TEXT NOT NULL,
	exit_code    INTEGER NOT NULL,
	success      INTEGER NOT NULL,
	output       TEXT NOT NULL,
	ran_at       TEXT NOT NULL,
	PRIMARY KEY (service_name, hook_type)
);
