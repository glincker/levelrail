package store

import "time"

// SharedEnvVar is one shared env var at the project, organization, or
// environment tier. Value is always empty when Secret is true: a
// secret-marked entry's plaintext never lives in project_env_vars/
// organization_env_vars/environment_env_vars, only its key does; the
// plaintext lives in service_secrets/service_secret_values instead, the
// same envelope-encryption tables every other platform-wide credential
// (Vault, registry, git source) already uses, under
// ProjectEnvSecretsKey/OrganizationEnvSecretsKey/EnvironmentEnvSecretsKey.
// UpdatedAt is the row's own updated_at (bumped on every SetX.../
// SetXSecretEnvVar write, so a plain override and a secret rotation
// alike move it), used by GET .../env/all to let a UI show a
// secret-marked entry's age and flag it stale, the same purpose
// SecretKeyInfo.UpdatedAt serves for per-app secrets.
type SharedEnvVar struct {
	Key       string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}
