package attention

import (
	"os"
	"strings"
	"time"
)

// EnvShortLivedCert tunes how short a certificate lifetime counts as
// auto-renewed internal issuance that never needs attention.
const EnvShortLivedCert = "APP_ATTENTION_SHORT_LIVED_CERT"

const defaultShortLivedCert = 7 * 24 * time.Hour

// Certificate sources, owners and states, as GET /api/v1/certificates
// reports them.
const (
	certSourceCustom   = "custom"
	certSourceProxy    = "proxy"
	certOwnerProxy     = "proxy"
	certOwnerInternal  = "internal"
	certStatusHealthy  = "healthy"
	certStatusExpired  = "expired"
	certRenewalStalled = "stalled"
)

// CertFacts is what the expiry rules decide from.
type CertFacts struct {
	Issuer    string
	Source    string
	TLSOwner  string
	Status    string
	Renewal   string
	NotBefore time.Time
	NotAfter  time.Time
	// Routed is false for a certificate whose domain no app or site serves.
	Routed bool
	// RenewalFailed is true when the CA refused the last renewal; NearExpiry
	// when an auto-renewed certificate is inside the failure window.
	RenewalFailed bool
	NearExpiry    bool
}

// ShortLivedCertLimit reads APP_ATTENTION_SHORT_LIVED_CERT.
func ShortLivedCertLimit() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvShortLivedCert)); err == nil && d > 0 {
		return d
	}
	return defaultShortLivedCert
}

// CertExpiryClass returns ClassIncident when the certificate's expiry needs
// an operator, else "". Internal-CA, proxy-owned and unrouted certificates
// never raise one, and an auto-renewing ACME certificate only does once it
// expired, its renewal stalled or failed, or it is inside the failure window. Uploaded certificates do not renew
// themselves, so they count as soon as they enter the warning window.
func CertExpiryClass(f CertFacts, shortLived time.Duration) string {
	if f.Status == "" || (f.Status == certStatusHealthy && !f.RenewalFailed && !f.NearExpiry) {
		return ""
	}
	switch {
	case f.Source == certSourceProxy || f.TLSOwner == certOwnerProxy:
		return ""
	case f.TLSOwner == certOwnerInternal || isInternalIssuer(f.Issuer):
		return ""
	case !f.NotBefore.IsZero() && f.NotAfter.Sub(f.NotBefore) <= shortLived:
		return ""
	case !f.Routed:
		return ""
	case f.Source == certSourceCustom:
		return ClassIncident
	case f.Status == certStatusExpired || f.Renewal == certRenewalStalled || f.RenewalFailed || f.NearExpiry:
		return ClassIncident
	}
	return ""
}

func isInternalIssuer(issuer string) bool {
	l := strings.ToLower(issuer)
	return strings.Contains(l, "local authority")
}
