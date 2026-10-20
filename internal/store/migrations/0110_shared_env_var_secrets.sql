-- Secret-backed shared env vars at the three existing shared-env tiers
-- (project_env_vars: migrations/0040, organization_env_vars:
-- migrations/0058, environment_env_vars: migrations/0060). Those
-- migrations deliberately scoped shared vars to plain values only,
-- pushing anything secret to an app's own SecretEnv; is_secret extends
-- that scope rather than replacing it: a secret-marked row's value
-- column stays empty ('') and its plaintext lives instead in
-- service_secrets/service_secret_values (migrations/0006) under
-- store.ProjectEnvSecretsKey/OrganizationEnvSecretsKey/
-- EnvironmentEnvSecretsKey, reusing internal/secrets.Manager rather than
-- a second encryption path.
ALTER TABLE project_env_vars ADD COLUMN is_secret INTEGER NOT NULL DEFAULT 0;
ALTER TABLE organization_env_vars ADD COLUMN is_secret INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environment_env_vars ADD COLUMN is_secret INTEGER NOT NULL DEFAULT 0;
