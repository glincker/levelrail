package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DomainTLSCertSecretsKey is the internal/secrets serviceName a domain's
// BYO TLS certificate and private key are stored under (envKeys
// DomainTLSCertCertificateEnvKey/DomainTLSCertPrivateKeyEnvKey),
// mirroring DomainBasicAuthSecretsKey's "<kind>/<domain>" shape.
func DomainTLSCertSecretsKey(domain string) string {
	return "domain-tls-cert/" + domain
}

// DomainTLSCertCertificateEnvKey and DomainTLSCertPrivateKeyEnvKey are
// the two fixed envKeys used within that namespace: one secrets-manager
// service slot per domain, two keys within it, sharing one
// data-encryption key (internal/secrets.Manager reuses a service's DEK
// across keys).
const (
	DomainTLSCertCertificateEnvKey = "certificate"
	DomainTLSCertPrivateKeyEnvKey  = "private_key"
)

// DomainTLSCert is one row of the domain_tls_cert table
// (migrations/0082_domain_tls_cert.sql): a domain already present in
// service_domains has an operator-supplied certificate uploaded. Neither
// the certificate nor its private key lives here: both go through
// internal/secrets under DomainTLSCertSecretsKey(domain), the same split
// DomainBasicAuth uses for its own password. ExpiresAt is stored in the
// clear (a certificate's NotAfter is not sensitive) so GET can report it
// without ever decrypting anything.
type DomainTLSCert struct {
	Domain     string
	UploadedAt time.Time
	ExpiresAt  time.Time
}

// GetDomainTLSCert returns the BYO cert row for domain. found is false
// when the domain has none uploaded, the default, valid state for any
// domain, not an error.
func (db *DB) GetDomainTLSCert(ctx context.Context, domain string) (DomainTLSCert, bool, error) {
	var d DomainTLSCert
	var uploadedAt, expiresAt string
	err := db.QueryRowContext(ctx, `
		SELECT domain, uploaded_at, expires_at FROM domain_tls_cert WHERE domain = ?
	`, domain).Scan(&d.Domain, &uploadedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DomainTLSCert{}, false, nil
	}
	if err != nil {
		return DomainTLSCert{}, false, fmt.Errorf("store: get domain tls cert: %w", err)
	}
	if d.UploadedAt, err = time.Parse(time.RFC3339Nano, uploadedAt); err != nil {
		return DomainTLSCert{}, false, fmt.Errorf("store: get domain tls cert: parse uploaded_at: %w", err)
	}
	if d.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
		return DomainTLSCert{}, false, fmt.Errorf("store: get domain tls cert: parse expires_at: %w", err)
	}
	return d, true, nil
}

// SetDomainTLSCert upserts the uploaded/expiry timestamps for domain's
// BYO certificate. Callers (internal/api) are expected to write the
// matching certificate and key through internal/secrets in the same
// request; this method only ever tracks metadata, never key material.
func (db *DB) SetDomainTLSCert(ctx context.Context, domain string, uploadedAt, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO domain_tls_cert (domain, uploaded_at, expires_at) VALUES (?, ?, ?)
		ON CONFLICT (domain) DO UPDATE SET uploaded_at = excluded.uploaded_at, expires_at = excluded.expires_at
	`, domain, uploadedAt.UTC().Format(time.RFC3339Nano), expiresAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set domain tls cert: %w", err)
	}
	return nil
}

// DeleteDomainTLSCert removes domain's BYO cert row, if any. Idempotent:
// deleting a domain with none uploaded is not an error.
func (db *DB) DeleteDomainTLSCert(ctx context.Context, domain string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM domain_tls_cert WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("store: delete domain tls cert: %w", err)
	}
	return nil
}

// ListDomainTLSCerts returns every domain_tls_cert row, ordered by
// domain, for the ingress controller to resolve fresh every reconcile
// pass, the same "never cache, re-derive from current state every call"
// convention ListDomainBasicAuth already follows.
func (db *DB) ListDomainTLSCerts(ctx context.Context) ([]DomainTLSCert, error) {
	rows, err := db.QueryContext(ctx, `SELECT domain, uploaded_at, expires_at FROM domain_tls_cert ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("store: list domain tls certs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DomainTLSCert
	for rows.Next() {
		var d DomainTLSCert
		var uploadedAt, expiresAt string
		if err := rows.Scan(&d.Domain, &uploadedAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("store: scan domain tls cert row: %w", err)
		}
		if d.UploadedAt, err = time.Parse(time.RFC3339Nano, uploadedAt); err != nil {
			return nil, fmt.Errorf("store: list domain tls certs: parse uploaded_at: %w", err)
		}
		if d.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
			return nil, fmt.Errorf("store: list domain tls certs: parse expires_at: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate domain tls cert rows: %w", err)
	}
	return out, nil
}
