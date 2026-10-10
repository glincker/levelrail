package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// CertStore is the store surface handleListCertificates and
// handleRenewDomainCertificate need: the two read methods
// internal/ingress.SQLiteStorage also uses, plus the delete method renew
// needs to clear a domain's stored certificate. *store.DB satisfies this
// structurally, same as every other Store sub-interface in this package.
type CertStore interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
	DeleteCertStorageValue(ctx context.Context, key string) error
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
	// the still-open real-ACME gap closes), purely informational.
	Issuer    string    `json:"issuer,omitempty"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	// Status is "healthy", "expiring_soon", or "expired", computed
	// purely from NotAfter vs now (alerting.CertExpiryStatus). A
	// kind=cert_expiry alert rule (internal/alerting/cert_expiry.go)
	// watches this same three-state signal proactively and can tell a
	// plain warning apart from a renewal that appears stalled; this
	// read-only endpoint only ever reports the current bucket, not that
	// stronger signal, since it has no notion of "across two reads."
	Status string `json:"status"`
	// Renewal is "ok" or "stalled". Stalled means the certificate is
	// expired, or has sat in expiring_soon with an unchanged NotAfter past
	// the stalled threshold (alerting.CertRenewalStates).
	Renewal string `json:"renewal"`
	// Apps is every app or static site that owns Domain, from
	// domainOwners; empty when Domain matches none of them (a certificate
	// for a hostname no longer configured on any app). The certificate
	// center page (web/src/routes/settings/certificates.tsx) uses this to
	// build the app-scoped renew/upload URL for this row.
	Apps []string `json:"apps,omitempty"`
	// Source is "custom" when domain_tls_cert has an operator-uploaded
	// certificate for Domain (handleSetDomainTLSCert), "acme" otherwise:
	// Caddy's automatic ACME/internal issuance. A "custom" certificate's
	// NotAfter/Status above still come from the stored leaf itself, same
	// computation either way; only the renew action's availability
	// differs (see handleRenewDomainCertificate).
	Source string `json:"source"`
	// ACMEFailure is the CA's last error for Domain when its most recent
	// issue or renewal attempt failed.
	ACMEFailure *acmeFailureResource `json:"acme_failure,omitempty"`
}

// CertObservationSource lists the expiry observations kind=cert_expiry
// rules record; *alerting.DB satisfies it.
type CertObservationSource interface {
	ListAllCertExpiryObservations(ctx context.Context) ([]alerting.CertExpiryObservation, error)
}

// handleListCertificates handles GET /api/v1/certificates: every
// certificate currently in internal/ingress's SQLite-backed
// certmagic.Storage (internal/ingress/certstorage.go),
// via alerting.ListCertificates, the same computation a kind=cert_expiry
// alert rule evaluates on its own tick. A control plane that has never
// issued a certificate returns an empty list, not an error: the same
// "absence is a real, reportable state, never a 5xx" shape
// handleSystemStatus already uses.
func (rt *Router) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	warningWindow := rt.certExpiryWarningWindow
	if warningWindow <= 0 {
		warningWindow = alerting.DefaultCertExpiryWarningWindow
	}

	infos, err := alerting.ListCertificates(r.Context(), rt.certs, warningWindow, time.Now(), rt.logger)
	if err != nil {
		rt.logger.Error("api: list certificates failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var obs []alerting.CertExpiryObservation
	if rt.certObservations != nil {
		var obsErr error
		if obs, obsErr = rt.certObservations.ListAllCertExpiryObservations(r.Context()); obsErr != nil {
			rt.logger.Warn("api: list cert expiry observations failed, reporting renewal from expiry only", slog.String("error", obsErr.Error()))
			obs = nil
		}
	}
	renewal := alerting.CertRenewalStates(infos, obs, rt.certRenewalStalledThreshold, time.Now())

	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list certificates: visibility", err)
		return
	}
	owners, err := rt.domainOwners(r.Context())
	if err != nil {
		rt.internalError(w, "api: list certificates: domain owners", err)
		return
	}
	customDomains, err := rt.customTLSCertDomains(r.Context())
	if err != nil {
		rt.internalError(w, "api: list certificates: custom tls cert domains", err)
		return
	}

	out := make([]certificateStatus, 0, len(infos))
	for _, info := range infos {
		if !certVisible(info.Domain, info.SANs, owners, canSee) {
			continue
		}
		source := "acme"
		if customDomains[strings.ToLower(info.Domain)] {
			source = "custom"
		}
		out = append(out, certificateStatus{
			Domain:    info.Domain,
			SANs:      info.SANs,
			Issuer:    info.Issuer,
			NotBefore: info.NotBefore,
			NotAfter:  info.NotAfter,
			Status:    info.Status,
			Renewal:   renewal[info.Domain],
			Apps:      owners[strings.ToLower(info.Domain)],
			Source:    source,

			ACMEFailure: acmeFailureFor(info.Domain),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// customTLSCertDomains returns the lowercased set of domains with an
// operator-uploaded (BYO) certificate currently on file
// (domain_tls_cert), for handleListCertificates' Source field.
func (rt *Router) customTLSCertDomains(ctx context.Context) (map[string]bool, error) {
	rows, err := rt.domainTLSCert.ListDomainTLSCerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list domain tls certs: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[strings.ToLower(row.Domain)] = true
	}
	return out, nil
}
