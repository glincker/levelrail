package store

// SharedEnvVar is one shared env var at the project, organization, or
// environment tier. Value is always empty when Secret is true: a
// secret-marked entry's plaintext never lives in project_env_vars/
// organization_env_vars/environment_env_vars, only its key does; the
// plaintext lives in service_secrets/service_secret_values instead, the
// same envelope-encryption tables every other platform-wide credential
// (Vault, registry, git source) already uses, under
// ProjectEnvSecretsKey/OrganizationEnvSecretsKey/EnvironmentEnvSecretsKey.
type SharedEnvVar struct {
	Key    string
	Value  string
	Secret bool
}
