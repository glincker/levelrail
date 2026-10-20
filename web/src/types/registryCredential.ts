// Wire types for the registry credential resource, matching
// internal/api/registry_credentials.go's registryCredentialResource /
// createRegistryCredentialRequest exactly. Password is write-only,
// present on the create request and nowhere else: never echoed back.

// RegistryCredentialExpiryStatus mirrors alerting.CertExpiryStatus's own
// three-state bucketing, reused server-side for this resource's
// expires_at too.
export type RegistryCredentialExpiryStatus =
  | 'healthy'
  | 'expiring_soon'
  | 'expired'

export interface RegistryCredential {
  id: string
  name: string
  registry_host: string
  username: string
  created_at: string
  // expires_at/expiry_status are operator-provided metadata: this
  // platform has no way to read an expiry out of an opaque credential
  // string. Both absent means no expiry was ever set.
  expires_at?: string
  expiry_status?: RegistryCredentialExpiryStatus
}

export interface CreateRegistryCredentialRequest {
  name: string
  registry_host: string
  username: string
  password: string
  expires_at?: string
}
