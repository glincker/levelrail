// Wire types for the registry credential resource, matching
// internal/api/registry_credentials.go's registryCredentialResource /
// createRegistryCredentialRequest exactly. Password is write-only,
// present on the create request and nowhere else: never echoed back.

export interface RegistryCredential {
  id: string
  name: string
  registry_host: string
  username: string
  created_at: string
}

export interface CreateRegistryCredentialRequest {
  name: string
  registry_host: string
  username: string
  password: string
}
