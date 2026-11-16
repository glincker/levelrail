-- Whether a sign-in with no other live session is held for approval (allow or hold); NULL means the env seed applies.
ALTER TABLE security_policy_settings ADD COLUMN approval_without_session TEXT;
