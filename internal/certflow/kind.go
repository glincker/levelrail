// Package certflow decides whether a domain is ready for a public
// certificate, tells operators the truth about the certificate it has, and
// retries stuck renewals with backoff. It holds no ACME client: issuance and
// renewal stay with the embedded ingress.
package certflow

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kind says who signed the certificate a domain serves.
type Kind string

// Kind values, from least to most trusted by browsers.
const (
	KindNone        Kind = "none"
	KindInternal    Kind = "internal"
	KindStaging     Kind = "staging"
	KindLetsEncrypt Kind = "letsencrypt"
	KindCustom      Kind = "custom"
	KindProxy       Kind = "proxy"
	KindOther       Kind = "other"
)

// Env names and defaults for the thresholds this package reads.
const (
	EnvFailDays = "APP_CERT_FAIL_DAYS"

	defaultFailDays = 14
)

var leIntermediate = regexp.MustCompile(`^(R|E)\d+$`)

// Source and owner strings match what GET /certificates reports.
const (
	sourceCustom = "custom"
	sourceProxy  = "proxy"
	ownerProxy   = "proxy"
	ownerIntern  = "internal"
)

// Classify names the signer from the leaf's issuer common name, the row's
// source and who owns TLS for the host.
func Classify(issuer, source, owner string) Kind {
	l := strings.ToLower(strings.TrimSpace(issuer))
	switch {
	case source == sourceCustom:
		return KindCustom
	case source == sourceProxy || owner == ownerProxy:
		return KindProxy
	case strings.Contains(l, "local authority"):
		return KindInternal
	case strings.Contains(l, "(staging)") || strings.Contains(l, "fake le"):
		return KindStaging
	case strings.Contains(l, "let's encrypt") || strings.Contains(l, "isrg") || leIntermediate.MatchString(strings.ToUpper(strings.TrimSpace(issuer))):
		return KindLetsEncrypt
	case owner == ownerIntern:
		return KindInternal
	case l == "":
		return KindNone
	}
	return KindOther
}

// RenewalResult is the outcome of the last automatic issue or renewal.
type RenewalResult string

// RenewalResult values.
const (
	RenewalOK     RenewalResult = "ok"
	RenewalFailed RenewalResult = "failed"
)

// Renewal is the last issue or renewal attempt as an operator reads it.
type Renewal struct {
	At     time.Time     `json:"at"`
	Result RenewalResult `json:"result"`
	Error  string        `json:"error,omitempty"`
}

// Badge is the per-domain certificate truth shown on lists and the overview.
type Badge struct {
	Kind        Kind     `json:"kind"`
	DaysLeft    int      `json:"days_left"`
	LastRenewal *Renewal `json:"last_renewal,omitempty"`
	// NextRetryAt is when the watchdog tries again, set only while a renewal
	// is stuck or failing.
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}

// DaysLeft is whole days until notAfter, negative once expired.
func DaysLeft(notAfter, now time.Time) int {
	return int(notAfter.Sub(now).Hours() / 24)
}

// BuildRenewal reports the last attempt: the CA's recorded failure when there
// is one, else the leaf's own issue time for certificates a CA signed.
func BuildRenewal(kind Kind, notBefore time.Time, failAt time.Time, failErr string, failed bool) *Renewal {
	switch {
	case failed:
		return &Renewal{At: failAt, Result: RenewalFailed, Error: failErr}
	case (kind == KindLetsEncrypt || kind == KindStaging || kind == KindOther) && !notBefore.IsZero():
		return &Renewal{At: notBefore, Result: RenewalOK}
	}
	return nil
}

// FailDays is how close to expiry an automatically renewed certificate turns
// the doctor red, from APP_CERT_FAIL_DAYS (default 14).
func FailDays() int {
	if n, err := strconv.Atoi(os.Getenv(EnvFailDays)); err == nil && n > 0 {
		return n
	}
	return defaultFailDays
}

// Trusted reports whether browsers accept certificates of this kind.
func (k Kind) Trusted() bool {
	return k == KindLetsEncrypt || k == KindCustom || k == KindProxy || k == KindOther
}

// AutoRenewed reports whether the platform renews this kind itself.
func (k Kind) AutoRenewed() bool {
	return k == KindLetsEncrypt || k == KindStaging || k == KindOther
}
