-- Setup wizard progress: the step the operator last viewed and a JSON
-- object mapping step id to "completed" or "skipped".
ALTER TABLE onboarding_state ADD COLUMN current_step TEXT NOT NULL DEFAULT '';
ALTER TABLE onboarding_state ADD COLUMN step_status TEXT NOT NULL DEFAULT '{}';
