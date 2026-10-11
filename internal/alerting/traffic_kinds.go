package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Domain and certificate rule kinds used by the traffic area. The spec's
// dns_drift, zone_not_delegated and proxy_route_error kinds are documented
// in docs/observability.md and not evaluated yet.
const (
	// KindCertExpiring fires while a certificate expires within Threshold
	// days (default DefaultCertExpiringDays). ResourceID "domain:<name>"
	// narrows it to one hostname; empty watches every certificate.
	KindCertExpiring Kind = "cert_expiring"
	// KindCertRenewalStalled fires when a renewal has not moved NotAfter for
	// the stalled threshold, or an ACME failure persists that long.
	KindCertRenewalStalled Kind = "cert_renewal_stalled"
	// KindDomainNotResolving fires when a domain has not resolved for
	// ForDuration (default DefaultDomainNotResolvingWindow). ResourceID
	// "service:<app>" narrows it to one app's domains.
	KindDomainNotResolving Kind = "domain_not_resolving"
)

// Defaults for the traffic kinds; a rule's own fields override them.
const (
	DefaultCertExpiringDays         = 14
	DefaultDomainNotResolvingWindow = 30 * time.Minute
)

// IsTrafficKind reports whether k is one of the traffic kinds above.
func IsTrafficKind(k Kind) bool {
	return k == KindCertExpiring || k == KindCertRenewalStalled || k == KindDomainNotResolving
}

// DomainListSource lists every configured domain. *store.DB satisfies it.
type DomainListSource interface {
	ListServiceDomains(ctx context.Context) ([]store.ServiceDomain, error)
}

// ACMEFailureSource reports whether the CA's last attempt for a domain failed.
type ACMEFailureSource interface {
	ACMEFailing(domain string) (errText string, failing bool)
}

// trafficKinds holds the traffic kinds' optional sources and the
// in-memory "since when" state their windows need.
type trafficKinds struct {
	domains DomainListSource
	acme    ACMEFailureSource

	mu    sync.Mutex
	since map[string]time.Time
}

// SetTrafficSources enables domain_not_resolving and the ACME half of
// cert_renewal_stalled. Either may be nil.
func (e *Engine) SetTrafficSources(domains DomainListSource, acme ACMEFailureSource) {
	e.traffic.domains, e.traffic.acme = domains, acme
}

// episode returns when key first became true, recording now when it just
// did, and forgets it when bad is false.
func (t *trafficKinds) episode(key string, bad bool, now time.Time) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.since == nil {
		t.since = map[string]time.Time{}
	}
	if !bad {
		delete(t.since, key)
		return now
	}
	if s, ok := t.since[key]; ok {
		return s
	}
	t.since[key] = now
	return now
}

func certMatchesSelector(info CertInfo, resourceID string) bool {
	want, ok := strings.CutPrefix(resourceID, "domain:")
	if !ok || want == "" || want == "*" {
		return true
	}
	want = strings.ToLower(want)
	if strings.EqualFold(info.Domain, want) {
		return true
	}
	for _, san := range info.SANs {
		if strings.EqualFold(san, want) {
			return true
		}
	}
	return false
}

// EvaluateCertExpiring runs one KindCertExpiring rule.
func EvaluateCertExpiring(ctx context.Context, certs CertSource, r Rule, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	days := int(r.Threshold)
	if days <= 0 {
		days = DefaultCertExpiringDays
	}
	window := time.Duration(days) * 24 * time.Hour
	infos, err := ListCertificates(ctx, certs, window, now, logger)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}
	next := r
	next.LastEvaluatedAt = &now
	var notices []string
	minDays := math.Inf(1)
	for _, info := range infos {
		if !certMatchesSelector(info, r.ResourceID) {
			continue
		}
		left := info.NotAfter.Sub(now).Hours() / 24
		minDays = math.Min(minDays, left)
		if info.NotAfter.Before(now.Add(window)) {
			notices = append(notices, fmt.Sprintf("%s: expires %s (%.0f days)", info.Domain, info.NotAfter.UTC().Format(time.DateOnly), left))
		}
	}
	if !math.IsInf(minDays, 1) {
		next.LastValue = &minDays
	}
	sort.Strings(notices)
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

// EvaluateCertRenewalStalled runs one KindCertRenewalStalled rule. It keeps
// its own expiry episodes (keyed by the rule ID) through EvaluateCertExpiry.
func (e *Engine) EvaluateCertRenewalStalled(ctx context.Context, r Rule, now time.Time) (Rule, []string, error) {
	threshold := e.certRenewalStalledThreshold
	if threshold <= 0 {
		threshold = DefaultCertRenewalStalledThreshold
	}
	window := e.certExpiryWarningWindow
	if window <= 0 {
		window = DefaultCertExpiryWarningWindow
	}
	if _, _, err := EvaluateCertExpiry(ctx, e.certs, e.rules, r, window, threshold, now, e.logger); err != nil {
		return r, nil, err
	}
	infos, err := ListCertificates(ctx, e.certs, window, now, e.logger)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}
	obs, err := e.rules.ListCertExpiryObservations(ctx, r.ID)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: load observations: %w", r.ID, err)
	}
	var notices []string
	for domain, state := range CertRenewalStates(infos, obs, threshold, now) {
		if state == CertRenewalStalled {
			notices = append(notices, domain+": renewal stalled, the certificate has not been renewed")
		}
	}
	notices = append(notices, e.persistentACMEFailures(ctx, r.ID, threshold, now)...)
	sort.Strings(notices)
	next := r
	next.LastEvaluatedAt = &now
	count := float64(len(notices))
	next.LastValue = &count
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

func (e *Engine) persistentACMEFailures(ctx context.Context, ruleID string, threshold time.Duration, now time.Time) []string {
	if e.traffic.acme == nil || e.traffic.domains == nil {
		return nil
	}
	domains, err := e.traffic.domains.ListServiceDomains(ctx)
	if err != nil {
		e.logger.Warn("alerting: cert_renewal_stalled: list domains failed", slog.String("rule_id", ruleID), slog.String("error", err.Error()))
		return nil
	}
	var out []string
	for _, d := range domains {
		errText, failing := e.traffic.acme.ACMEFailing(d.Domain)
		since := e.traffic.episode(ruleID+"|acme|"+d.Domain, failing, now)
		if failing && now.Sub(since) >= threshold {
			out = append(out, fmt.Sprintf("%s: certificate request failing since %s: %s", d.Domain, since.UTC().Format(time.RFC3339), errText))
		}
	}
	return out
}

// EvaluateDomainNotResolving runs one KindDomainNotResolving rule.
func (e *Engine) EvaluateDomainNotResolving(ctx context.Context, r Rule, now time.Time) (Rule, []string, error) {
	window := r.ForDuration
	if window <= 0 {
		window = DefaultDomainNotResolvingWindow
	}
	domains, err := e.traffic.domains.ListServiceDomains(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list domains: %w", r.ID, err)
	}
	app, scoped := strings.CutPrefix(r.ResourceID, "service:")
	var notices []string
	for _, d := range domains {
		if scoped && app != "" && d.ServiceName != app {
			continue
		}
		status, err := e.domainChecker.CheckDomainStatus(ctx, d.Domain)
		if err != nil {
			continue
		}
		bad := status == domainHealthStatusNotResolving
		since := e.traffic.episode(r.ID+"|dns|"+d.Domain, bad, now)
		if bad && now.Sub(since) >= window {
			notices = append(notices, fmt.Sprintf("%s: not resolving for %s", d.Domain, now.Sub(since).Round(time.Minute)))
		}
	}
	sort.Strings(notices)
	next := r
	next.LastEvaluatedAt = &now
	count := float64(len(notices))
	next.LastValue = &count
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

// evaluateTrafficKind dispatches a traffic kind; skip is true when the
// sources it needs are not configured on this control plane.
func (e *Engine) evaluateTrafficKind(ctx context.Context, r Rule, now time.Time) (next Rule, notices []string, skip bool, err error) {
	switch r.Kind {
	case KindCertExpiring:
		if e.certs == nil {
			return r, nil, true, nil
		}
		next, notices, err = EvaluateCertExpiring(ctx, e.certs, r, now, e.logger)
	case KindCertRenewalStalled:
		if e.certs == nil {
			return r, nil, true, nil
		}
		next, notices, err = e.EvaluateCertRenewalStalled(ctx, r, now)
	case KindDomainNotResolving:
		if e.traffic.domains == nil || e.domainChecker == nil {
			return r, nil, true, nil
		}
		if !e.domainHealthThrottle.ready(r.ID, e.domainHealthCheckInterval, now) {
			return r, nil, true, nil
		}
		next, notices, err = e.EvaluateDomainNotResolving(ctx, r, now)
	}
	return next, notices, false, err
}
