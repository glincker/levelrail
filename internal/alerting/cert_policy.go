package alerting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Defaults for the cert expiry policy, each overridable through the environment.
const (
	DefaultCertLastResortWindow = 72 * time.Hour
	DefaultCertReminderInterval = 24 * time.Hour
	// DefaultCertRenewalRatio mirrors the fraction of lifetime left at which the ingress renews.
	DefaultCertRenewalRatio = 1.0 / 3.0

	envCertLastResortHours  = "APP_ALERT_CERT_LAST_RESORT_HOURS"
	envCertReminderInterval = "APP_ALERT_CERT_REMINDER_INTERVAL"
	envCertRenewalRatio     = "APP_ALERT_CERT_RENEW_RATIO"
)

// Fix actions a cert alert can offer.
const (
	CertActionRenew = "renew"
	CertActionDNS   = "dns"
)

// CertExpiryPolicy decides which certificates a cert_expiry rule reports. The zero value keeps the legacy behavior of reporting every non-healthy certificate.
type CertExpiryPolicy struct {
	Enabled bool
	// Windows lists every enabled cert_expiry rule window so only the tightest matching one reports a certificate.
	Windows    []time.Duration
	LastResort time.Duration
	RenewRatio float64
	// ReminderInterval is the minimum gap between reminders for a still-firing rule.
	ReminderInterval time.Duration
	ACME             ACMEFailureSource
	// BaseURL is the instance's public dashboard address used for email deep links.
	BaseURL string
}

// CertFact is the structured form of one cert alert notice, used by the email and the Inbox card.
type CertFact struct {
	Domain   string
	Issuer   string
	DaysLeft int
	Reason   string
	Action   string
}

// CertPolicyFromEnv reads the policy tunables; invalid values keep the defaults.
func CertPolicyFromEnv(lookup func(string) string) CertExpiryPolicy {
	p := CertExpiryPolicy{
		Enabled: true, LastResort: DefaultCertLastResortWindow,
		RenewRatio: DefaultCertRenewalRatio, ReminderInterval: DefaultCertReminderInterval,
	}
	if h, err := strconv.ParseFloat(strings.TrimSpace(lookup(envCertLastResortHours)), 64); err == nil && h >= 0 {
		p.LastResort = time.Duration(h * float64(time.Hour))
	}
	if d, err := time.ParseDuration(strings.TrimSpace(lookup(envCertReminderInterval))); err == nil && d > 0 {
		p.ReminderInterval = d
	}
	if r, err := strconv.ParseFloat(strings.TrimSpace(lookup(envCertRenewalRatio)), 64); err == nil && r > 0 && r < 1 {
		p.RenewRatio = r
	}
	return p
}

// tightest returns the smallest window the remaining time still falls inside.
func (p CertExpiryPolicy) tightest(left time.Duration) time.Duration {
	best := time.Duration(0)
	for _, w := range p.Windows {
		if left < w && (best == 0 || w < best) {
			best = w
		}
	}
	return best
}

// assess reports whether a human needs to act on info, and why.
func (p CertExpiryPolicy) assess(info CertInfo, now time.Time, grace time.Duration, stalled bool) (CertFact, bool) {
	left := info.NotAfter.Sub(now)
	fact := CertFact{Domain: info.Domain, Issuer: info.Issuer, DaysLeft: int(math.Floor(left.Hours() / 24)), Action: CertActionRenew}
	if info.Internal {
		return fact, false
	}
	if p.ACME != nil {
		if msg, failing := p.ACME.ACMEFailing(info.Domain); failing {
			fact.Reason = "Renewal is failing: " + msg
			if looksLikeDNSFailure(msg) {
				fact.Action = CertActionDNS
			}
			return fact, true
		}
	}
	switch {
	case left <= p.LastResort:
		fact.Reason = "The certificate is inside the last-resort window and has not been renewed"
	case stalled:
		fact.Reason = "Renewal appears stalled: the expiry date has not moved"
	default:
		lifetime := info.NotAfter.Sub(info.NotBefore)
		renewAt := info.NotAfter.Add(-time.Duration(float64(lifetime) * p.RenewRatio))
		if now.Before(renewAt.Add(grace)) {
			return fact, false
		}
		fact.Reason = "Automatic renewal is overdue"
	}
	return fact, true
}

func looksLikeDNSFailure(msg string) bool {
	m := strings.ToLower(msg)
	for _, k := range []string{"dns", "nxdomain", "no such host", "no valid a", "no valid aaaa", "resolve"} {
		if strings.Contains(m, k) {
			return true
		}
	}
	return false
}

// Notice renders the fact as the one-line notice shown in history, webhooks and the plain-text email.
func (f CertFact) Notice() string {
	when := fmt.Sprintf("expires in %d days", f.DaysLeft)
	switch {
	case f.DaysLeft < 0:
		when = "has expired"
	case f.DaysLeft == 0:
		when = "expires today"
	case f.DaysLeft == 1:
		when = "expires in 1 day"
	}
	issuer := f.Issuer
	if issuer == "" {
		issuer = "unknown"
	}
	return fmt.Sprintf("%s: %s (issuer %s). %s.", f.Domain, when, issuer, f.Reason)
}
