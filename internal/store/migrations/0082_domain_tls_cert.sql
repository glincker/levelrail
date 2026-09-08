-- BYO (bring your own) TLS certificate upload: an operator can supply
-- their own certificate and private key for a domain instead of relying
-- on Caddy's automatic ACME/internal issuance (internal/reconcile/ingress),
-- for domains ACME cannot reach: internal-only hosts, externally issued
-- wildcards, or a cert already provisioned before DNS cuts over. Mirrors
-- domain_basic_auth (0053) and domain_maintenance (0080): presence of a
-- row means enabled. The certificate and private key themselves live in
-- internal/secrets under store.DomainTLSCertSecretsKey(domain), the same
-- envelope-encryption split domain_basic_auth uses for its own password;
-- expires_at is stored here in the clear (a certificate's own NotAfter is
-- not sensitive) so GET can report it without decrypting anything.
CREATE TABLE domain_tls_cert (
	domain      TEXT PRIMARY KEY REFERENCES service_domains(domain) ON DELETE CASCADE,
	uploaded_at TEXT NOT NULL,
	expires_at  TEXT NOT NULL
);
