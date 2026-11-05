// Optional result the provider settings PUT returns after testing a saved
// credential (internal/api/dns_verify.go). Never carries the credential.
export interface DnsTokenVerification {
  ok?: boolean
  missing_scopes?: string[]
  zones_visible?: number
  message?: string
}
