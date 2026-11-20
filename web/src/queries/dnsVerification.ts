// Optional result the provider settings PUT returns after testing a saved
// credential (internal/api/dns_verify.go). Never carries the credential.
export interface DnsTokenVerification {
  ok?: boolean
  missing_scopes?: string[]
  zones_visible?: number
  message?: string
  // Route 53 only: each probed action and the minimal policy for denied ones.
  permissions?: { action: string; state: 'allowed' | 'denied' | 'unknown' }[]
  iam_policy?: string
}
