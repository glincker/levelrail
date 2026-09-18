-- vault_env: JSON object, envVarName -> {"path":"...","key":"..."}, the
-- persisted form of app.yaml's { vault: { path, key } } env var syntax
-- (internal/spec.EnvVar.Vault), mutually exclusive per env var with
-- secret_env and database_env. Same storage shape database_env already
-- uses (migrations/0050) and the same "ordinary desired state,
-- SaveDesiredService writes it on every save" treatment: a declaration
-- derived fresh from app.yaml each deploy, never a value itself.
ALTER TABLE desired_services ADD COLUMN vault_env TEXT NOT NULL DEFAULT '{}';
