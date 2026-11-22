package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DefaultDNSCredentialRecheck is how long a provider check stays fresh when the
// rule sets no ForDuration, so the engine tick never hammers provider APIs.
const DefaultDNSCredentialRecheck = 6 * time.Hour

// DNSCredentialFixPath is the settings page where a DNS credential is replaced.
const DNSCredentialFixPath = "/settings/dns" //nolint:gosec // a page path, not a credential

// DNSCredentialHealth is one connected DNS provider's latest access check.
type DNSCredentialHealth struct {
	Provider string
	// Outcome is "ok", "invalid", "denied", "unreachable" or "error".
	Outcome string
	Missing []string
}

// DNSCredentialSource re-verifies stored DNS credentials, reusing a result
// younger than maxAge.
type DNSCredentialSource interface {
	DNSCredentialHealth(ctx context.Context, maxAge time.Duration) ([]DNSCredentialHealth, error)
}

// EvaluateDNSCredentialFailing runs one KindDNSCredentialFailing rule. Only a
// revoked or under-scoped credential fires; an unreachable provider does not.
func EvaluateDNSCredentialFailing(ctx context.Context, source DNSCredentialSource, r Rule, now time.Time) (Rule, string, error) {
	recheck := r.ForDuration
	if recheck <= 0 {
		recheck = DefaultDNSCredentialRecheck
	}
	creds, err := source.DNSCredentialHealth(ctx, recheck)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read dns credential health: %w", r.ID, err)
	}
	var bad []string
	for _, c := range creds {
		switch c.Outcome {
		case "invalid":
			bad = append(bad, c.Provider+": credential rejected as invalid, revoked or expired")
		case "denied":
			line := c.Provider + ": credential lacks zone rights"
			if len(c.Missing) > 0 {
				line += " (" + strings.Join(c.Missing, ", ") + ")"
			}
			bad = append(bad, line)
		}
	}
	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(bad))
	next.LastValue = &v
	var notice string
	if len(bad) > 0 {
		notice = "DNS provider credential failing, certificate renewals that use it will fail: " + strings.Join(bad, "; ") + ". Fix: open " + DNSCredentialFixPath + " and save a working credential"
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}
