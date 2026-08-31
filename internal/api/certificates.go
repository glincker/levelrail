package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// CertStore is the store surface handleListCertificates needs: the same
// two read methods internal/ingress.SQLiteStorage already calls through
// its own CertStore interface, narrowed further since this handler never
// writes, deletes, or locks. *store.DB satisfies this structurally, the
// same consumer-defined-interface convention every other Store
// sub-interface in this package follows. Identical in shape to
// alerting.CertSource (a kind=cert_expiry rule's own read surface): the
// certificate-listing computation itself lives once, in
// alerting.ListCertificates, and this handler just maps its result to
// the wire shape below, so the dashboard's TLS card and an alert rule
// can never silently disagree about a certificate's status.
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
	// purely from NotAfter vs now (alerting.CertExpiryStatus). A
	// kind=cert_expiry alert rule (internal/alerting/cert_expiry.go)
	// watches this same three-state signal proactively and can tell a
	// plain warning apart from a renewal that appears stalled; this
	// read-only endpoint only ever reports the current bucket, not that
	// stronger signal, since it has no notion of "across two reads."
	Status string `json:"status"`
}

// handleListCertificates handles GET /api/v1/certificates: every
// certificate currently in internal/ingress's SQLite-backed
// certmagic.Storage (internal/ingress/certstorage.go, TASKS.md 3.6),
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

	out := make([]certificateStatus, 0, len(infos))
	for _, info := range infos {
		out = append(out, certificateStatus{
			Domain:    info.Domain,
			SANs:      info.SANs,
			Issuer:    info.Issuer,
			NotBefore: info.NotBefore,
			NotAfter:  info.NotAfter,
			Status:    info.Status,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
