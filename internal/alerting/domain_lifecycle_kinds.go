package alerting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Domain lifecycle rule kinds. Both are explicit opt-in: nothing creates them automatically.
const (
	// KindDomainTrafficDrop fires when a domain's requests in the last window fall below
	// Threshold percent of the same window a day earlier. ResourceID "service:<app>" watches
	// that app's domains, "domain:<name>" one domain. ForDuration is the window,
	// RestartCountThreshold the minimum baseline request count.
	KindDomainTrafficDrop Kind = "domain_traffic_drop"
	// KindParkedDomainCertExpiry fires for parked domains whose certificate has fewer than
	// Threshold days left. Platform-wide.
	KindParkedDomainCertExpiry Kind = "parked_domain_cert_expiry"
)

// Defaults for the domain lifecycle kinds; a rule's own fields override them.
const (
	DefaultDomainTrafficDropWindow      = time.Hour
	DefaultDomainTrafficDropPercent     = 50.0
	DefaultDomainTrafficDropMinBaseline = 20
	DefaultParkedCertExpiryDays         = 14
	// domainTrafficBaselineShift is how far back the comparison window sits.
	domainTrafficBaselineShift = 24 * time.Hour
)

// DomainTrafficSource counts requests for one domain of an app in a time range.
// Implementations read the visitor rollups and live ring, never raw logs.
type DomainTrafficSource interface {
	DomainRequests(ctx context.Context, app, domain string, since, until time.Time) (int, bool)
}

// ParkedDomainSource lists parked domains. *store.DB satisfies it.
type ParkedDomainSource interface {
	ListParkedDomains(ctx context.Context) ([]store.ParkedDomain, error)
}

// SetDomainTraffic enables domain_traffic_drop; nil leaves the kind skipped.
func (e *Engine) SetDomainTraffic(src DomainTrafficSource) { e.traffic.visits = src }

// DomainTrafficDropParams returns the rule's effective window, percent and minimum baseline.
func DomainTrafficDropParams(r Rule) (window time.Duration, percent float64, minBaseline int) {
	window, percent, minBaseline = r.ForDuration, r.Threshold, r.RestartCountThreshold
	if window <= 0 {
		window = DefaultDomainTrafficDropWindow
	}
	if percent <= 0 {
		percent = DefaultDomainTrafficDropPercent
	}
	if minBaseline <= 0 {
		minBaseline = DefaultDomainTrafficDropMinBaseline
	}
	return window, percent, minBaseline
}

// EvaluateDomainTrafficDrop runs one KindDomainTrafficDrop rule. Missing data never fires.
func (e *Engine) EvaluateDomainTrafficDrop(ctx context.Context, r Rule, now time.Time) (Rule, []string, error) {
	next := r
	next.LastEvaluatedAt = &now
	window, percent, minBaseline := DomainTrafficDropParams(r)
	domains, err := e.traffic.domains.ListServiceDomains(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list domains: %w", r.ID, err)
	}
	app, scopedApp := strings.CutPrefix(r.ResourceID, "service:")
	one, scopedDomain := strings.CutPrefix(r.ResourceID, "domain:")
	var notices []string
	for _, d := range domains {
		if scopedApp && app != "" && d.ServiceName != app {
			continue
		}
		if scopedDomain && one != "" && !strings.EqualFold(d.Domain, one) {
			continue
		}
		baseEnd := now.Add(-domainTrafficBaselineShift)
		base, okBase := e.traffic.visits.DomainRequests(ctx, d.ServiceName, d.Domain, baseEnd.Add(-window), baseEnd)
		cur, okCur := e.traffic.visits.DomainRequests(ctx, d.ServiceName, d.Domain, now.Add(-window), now)
		if !okBase || !okCur || base < minBaseline {
			continue
		}
		if float64(cur) < float64(base)*percent/100 {
			notices = append(notices, fmt.Sprintf("%s: %d requests in the last %s, down from %d the same time yesterday", d.Domain, cur, window, base))
		}
	}
	sort.Strings(notices)
	count := float64(len(notices))
	next.LastValue = &count
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

func certCoversDomain(info CertInfo, domain string) bool {
	match := func(name string) bool {
		if strings.EqualFold(name, domain) {
			return true
		}
		suffix, ok := strings.CutPrefix(strings.ToLower(name), "*.")
		if !ok {
			return false
		}
		host := strings.ToLower(domain)
		rest, found := strings.CutSuffix(host, "."+suffix)
		return found && rest != "" && !strings.Contains(rest, ".")
	}
	if match(info.Domain) {
		return true
	}
	for _, san := range info.SANs {
		if match(san) {
			return true
		}
	}
	return false
}

// EvaluateParkedDomainCertExpiry runs one KindParkedDomainCertExpiry rule.
// A parked domain with no certificate is not an expiry, so it never fires.
func (e *Engine) EvaluateParkedDomainCertExpiry(ctx context.Context, r Rule, now time.Time) (Rule, []string, error) {
	days := int(r.Threshold)
	if days <= 0 {
		days = DefaultParkedCertExpiryDays
	}
	parked, err := e.traffic.parked.ListParkedDomains(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list parked domains: %w", r.ID, err)
	}
	next := r
	next.LastEvaluatedAt = &now
	if len(parked) == 0 {
		return advanceState(next, r, false, 0, now), nil, nil
	}
	window := time.Duration(days) * 24 * time.Hour
	infos, err := ListCertificates(ctx, e.certs, window, now, e.logger)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}
	var notices []string
	for _, p := range parked {
		var best *CertInfo
		for i := range infos {
			if certCoversDomain(infos[i], p.Domain) && (best == nil || infos[i].NotAfter.After(best.NotAfter)) {
				best = &infos[i]
			}
		}
		if best == nil {
			continue
		}
		left := best.NotAfter.Sub(now).Hours() / 24
		if left < float64(days) {
			notices = append(notices, fmt.Sprintf("%s (parked): certificate expires %s (%.0f days)", p.Domain, best.NotAfter.UTC().Format(time.DateOnly), left))
		}
	}
	sort.Strings(notices)
	count := float64(len(notices))
	next.LastValue = &count
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}
