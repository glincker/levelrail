package alerting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultDomainHealthCheckInterval is how often a domain_health rule runs a real DNS check by default.
// Checking more often than Engine's 30s tick only adds outbound DNS traffic.
const DefaultDomainHealthCheckInterval = 5 * time.Minute

// Duplicated from internal/api, which already imports alerting, so importing back would cycle.
// unconfigured is not unhealthy: it means no APP_PUBLIC_HOST, so the check is inconclusive.
const (
	domainHealthStatusNotResolving      = "not_resolving"
	domainHealthStatusResolvesElsewhere = "resolves_elsewhere"
)

// DomainCheckSource is the narrow DNS check surface EvaluateDomainHealth needs, shared with the dashboard's Check now button.
type DomainCheckSource interface {
	CheckDomainStatus(ctx context.Context, domain string) (status string, err error)
}

// AppDomainSource is the narrow store surface EvaluateDomainHealth needs:
// which domains a domain_health rule's own app currently has configured.
// *store.DB satisfies this structurally.
type AppDomainSource interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// Real DNS calls cost network time, unlike other rule kinds' local reads, so checks are throttled below Engine's tick.
// The throttle is lost on restart, which is accepted.
type domainHealthThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time // rule ID -> last real check time
}

func newDomainHealthThrottle() *domainHealthThrottle {
	return &domainHealthThrottle{last: make(map[string]time.Time)}
}

// ready reports whether ruleID is due for a check and records now when it is.
// A rule never seen before is due immediately, so a restart does not wait out an interval.
func (t *domainHealthThrottle) ready(ruleID string, interval time.Duration, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.last[ruleID]; ok && now.Sub(last) < interval {
		return false
	}
	t.last[ruleID] = now
	return true
}

// domainHealthAppName extracts the app name from a service-scoped ResourceID.
func domainHealthAppName(resourceID string) (name string, ok bool) {
	return strings.CutPrefix(resourceID, "service:")
}

// EvaluateDomainHealth runs one domain_health rule against every domain of the app in its ResourceID.
// An app with no domains, or one since deleted, is treated as no data: it neither fires nor resolves.
func EvaluateDomainHealth(ctx context.Context, apps AppDomainSource, checker DomainCheckSource, r Rule, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if logger == nil {
		logger = slog.Default()
	}

	next := r
	next.LastEvaluatedAt = &now

	appName, ok := domainHealthAppName(r.ResourceID)
	if !ok {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, nil, nil
	}

	svc, err := apps.GetDesiredService(ctx, appName)
	if errors.Is(err, store.ErrServiceNotFound) {
		// The watched app was deleted: go quiet instead of erroring on every tick.
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, nil, nil
	}
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: load app %q: %w", r.ID, appName, err)
	}
	if len(svc.Domains) == 0 {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, nil, nil
	}

	var (
		notices      []string
		anyUnhealthy bool
		unhealthy    float64
	)
	for _, domain := range svc.Domains {
		status, err := checker.CheckDomainStatus(ctx, domain)
		if err != nil {
			logger.Warn("alerting: evaluate domain_health rule: check domain failed, skipping domain",
				slog.String("rule_id", r.ID), slog.String("domain", domain), slog.String("error", err.Error()))
			continue
		}
		if status != domainHealthStatusNotResolving && status != domainHealthStatusResolvesElsewhere {
			continue
		}
		anyUnhealthy = true
		unhealthy++
		notices = append(notices, domainHealthNotice(domain, status))
	}

	next.LastValue = &unhealthy

	return advanceState(next, r, anyUnhealthy, r.ForDuration, now), notices, nil
}

func domainHealthNotice(domain, status string) string {
	switch status {
	case domainHealthStatusNotResolving:
		return fmt.Sprintf("%s: not resolving (no DNS record found)", domain)
	case domainHealthStatusResolvesElsewhere:
		return fmt.Sprintf("%s: resolves elsewhere (DNS points somewhere other than this control plane)", domain)
	default:
		return fmt.Sprintf("%s: %s", domain, status)
	}
}
