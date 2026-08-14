package api

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// certStorageCertsPrefix is the top-level storage-key prefix
// certmagic.KeyBuilder (github.com/caddyserver/certmagic, storage.go)
// always nests every certificate under, regardless of issuer:
// "certificates/<issuer-key>/<domain>/<domain>.crt" (plus sibling
// ".key"/".json" files this handler skips, see handleListCertificates).
// Duplicated here as a literal rather than imported from certmagic, the
// same "internal/store never depends on certmagic, only internal/ingress
// does" boundary internal/store/cert_storage.go's own
// CertStorageKeyInfo doc comment already draws: internal/api shouldn't
// need to pull in Caddy just to read a key-space convention.
const certStorageCertsPrefix = "certificates"

// defaultCertExpiryWarningWindow is how far ahead of a certificate's
// NotAfter handleListCertificates starts flagging it "expiring_soon"
// instead of "healthy" (see statusForExpiry). 14 days: comfortably ahead
// of Caddy's own automatic-renewal window (certmagic starts renewing a
// certificate at roughly a third of its remaining lifetime, ~30 days out
// for a 90-day Let's Encrypt cert) so an operator sees the warning
// before, not after, the point Caddy itself would already be retrying.
const defaultCertExpiryWarningWindow = 14 * 24 * time.Hour

// CertStore is the store surface handleListCertificates needs: the same
// two read methods internal/ingress.SQLiteStorage already calls through
// its own CertStore interface, narrowed further since this handler never
// writes, deletes, or locks. *store.DB satisfies this structurally, the
// same consumer-defined-interface convention every other Store
// sub-interface in this package follows.
type CertStore interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
}

// certificateStatus is one issued certificate's wire shape: what
// settings/general.tsx's TLS card (this project treats "a cert
// renewal fails silently at 3am" as its main risk to catch
// before it bites a real user) needs to show an operator before an
// expiry becomes an outage, no more.
type certificateStatus struct {
	// Domain is the certificate's primary subject: the leaf's first SAN
	// if it has any, else its CommonName, else (only if a stored leaf
	// somehow has neither) the storage key's own domain path segment, so
	// a malformed-but-present entry still gets a row instead of silently
	// vanishing from the list. See certDomain.
	Domain string `json:"domain"`
	// SANs is every subject alternative name on the leaf, Domain
	// included, for a certificate that covers more than one hostname.
	SANs []string `json:"sans,omitempty"`
	// Issuer is the leaf's issuer CommonName ("Test CA", the internal
	// issuer's own self-signed CA name, or a real ACME CA's name once
	// TASKS.md's still-open real-ACME gap closes), purely informational.
	Issuer    string    `json:"issuer,omitempty"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	// Status is "healthy", "expiring_soon", or "expired", computed
	// purely from NotAfter vs now (statusForExpiry). This codebase's TLS
	// automation policy (internal/ingress/config.go's
	// internalIssuerTLSApp) uses only Caddy's offline "internal" issuer
	// today; real ACME is explicitly unverified per ADR 005. That means
	// there is no last-renewal-attempt or last-renewal-error signal
	// anywhere in this codebase yet to surface alongside expiry, and
	// none is added speculatively here: expiry is the one honest
	// renewal-health signal available without reaching into cert
	// issuance/renewal logic itself, which is out of scope for a
	// read-only status endpoint.
	Status string `json:"status"`
}

// statusForExpiry buckets notAfter, relative to now, into the three
// states settings/general.tsx's Badge renders (success/warning/
// destructive): "expired" once notAfter has passed or is passing this
// instant, "expiring_soon" within warningWindow of it, "healthy"
// otherwise. A notAfter exactly warningWindow away is still "healthy":
// the window is when to start warning, not an inclusive boundary.
func statusForExpiry(notAfter, now time.Time, warningWindow time.Duration) string {
	switch {
	case !notAfter.After(now):
		return "expired"
	case notAfter.Before(now.Add(warningWindow)):
		return "expiring_soon"
	default:
		return "healthy"
	}
}

// handleListCertificates handles GET /api/v1/certificates: every
// certificate currently in internal/ingress's SQLite-backed
// certmagic.Storage (internal/ingress/certstorage.go, TASKS.md 3.6),
// parsed from its own stored PEM bytes rather than requiring any new
// tracking to be added first, the same "read status data and shape it
// into a response" precedent handleGetNodeHealth (nodes.go) already
// establishes. A control plane that has never issued a certificate
// returns an empty list, not an error: the same "absence is a real,
// reportable state, never a 5xx" shape handleSystemStatus already uses.
func (rt *Router) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	keys, err := rt.certs.ListCertStorageKeys(r.Context(), certStorageCertsPrefix, true)
	if err != nil {
		rt.logger.Error("api: list certificates: list storage keys failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	warningWindow := rt.certExpiryWarningWindow
	if warningWindow <= 0 {
		warningWindow = defaultCertExpiryWarningWindow
	}
	now := time.Now()

	out := make([]certificateStatus, 0, len(keys))
	for _, key := range keys {
		// Every certmagic site prefix also holds a ".key" (private key)
		// and a ".json" (ACME certificate-resource metadata) entry
		// alongside the ".crt" this handler wants; skipping anything
		// that isn't a stored certificate is what keeps this a per-
		// certificate list instead of three rows per domain.
		if path.Ext(key) != ".crt" {
			continue
		}

		v, err := rt.certs.GetCertStorageValue(r.Context(), key)
		if err != nil {
			// A key that ListCertStorageKeys just returned but is now
			// missing (deleted between the two calls, e.g. a
			// concurrent renewal) is a stale read, not a request
			// failure: log it and move on, the same "one broken entry
			// must not block the rest" shape handleDrainNode already
			// applies to a bulk operation.
			if !errors.Is(err, store.ErrCertStorageKeyNotFound) {
				rt.logger.Warn("api: list certificates: load stored certificate failed, skipping",
					slog.String("key", key), slog.String("error", err.Error()))
			}
			continue
		}
		cert, err := leafCertificate(v.Value)
		if err != nil {
			rt.logger.Warn("api: list certificates: parse stored certificate failed, skipping",
				slog.String("key", key), slog.String("error", err.Error()))
			continue
		}
		out = append(out, certificateStatus{
			Domain:    certDomain(cert, key),
			SANs:      cert.DNSNames,
			Issuer:    cert.Issuer.CommonName,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			Status:    statusForExpiry(cert.NotAfter, now, warningWindow),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	writeJSON(w, http.StatusOK, out)
}

// leafCertificate parses the first certificate in a PEM-encoded chain,
// the leaf: a stored ".crt" value is the full chain, leaf certificate
// first, the same shape certmagic itself reads back when loading a
// managed certificate.
func leafCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("api: no PEM block found in stored certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// certDomain picks a stored certificate's primary subject: the first SAN
// if it has any (the normal case, every certificate this codebase issues
// carries at least one DNS SAN), else its CommonName, else the domain
// segment of its own storage key
// (certmagic.KeyBuilder.SiteCert's "certificates/<issuer>/<domain>/
// <domain>.crt" shape), so a pathological certificate with neither still
// gets a usable label instead of an empty string.
func certDomain(cert *x509.Certificate, key string) string {
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	return path.Base(path.Dir(key))
}
