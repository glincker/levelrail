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

// DefaultDomainHealthCheckInterval is how often a kind=domain_health rule
// performs a real DNS check, independent of Engine's own tick cadence, when
// no override is configured. The failure mode this rule exists to catch (a
// CNAME silently repointed away) unfolds over months, so checking far more
// often than Engine's 30s tick buys nothing but outbound DNS traffic; five
// minutes still catches drift long before "six months later."
const DefaultDomainHealthCheckInterval = 5 * time.Minute

// Domain check status values: duplicated from internal/api's own
// domainCheckStatus* constants (domain_check.go) rather than imported,
// since internal/api imports internal/alerting already, so the reverse
// import would cycle, the same reasoning CertSource's own doc comment
// gives for duplicating its method set. "unconfigured" is deliberately
// not among the unhealthy statuses below: it means the control plane has
// no APP_PUBLIC_HOST configured at all, not that a domain's DNS is wrong,
// and is treated as inconclusive, the same "no recent data" stance
// EvaluateThreshold takes.
const (
	domainHealthStatusNotResolving      = "not_resolving"
	domainHealthStatusResolvesElsewhere = "resolves_elsewhere"
)

// DomainCheckSource is the narrow surface EvaluateDomainHealth needs: the
// same DNS check GET /api/v1/apps/{name}/domains/{domain}/check itself
// runs (internal/api's runDomainCheck), reused through this interface
// rather than reimplemented here, so the dashboard's "Check now" button
// and a domain_health rule can never silently disagree on what
// "connected" means. *api.Router satisfies this via CheckDomainStatus.
type DomainCheckSource interface {
	CheckDomainStatus(ctx context.Context, domain string) (status string, err error)
}

// AppDomainSource is the narrow store surface EvaluateDomainHealth needs:
// which domains a domain_health rule's own app currently has configured.
// *store.DB satisfies this structurally.
type AppDomainSource interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// domainHealthThrottle rate-limits real domain_health checks below
// Engine's own tick interval: unlike every other rule kind, checking a
// domain's DNS is a real outbound network call, not a local metrics/store
// read, so there is real cost to paying it on every tick. Lost on a
// control plane restart, the same accepted tradeoff domainCheckCache
// (internal/api/domain_check.go) already makes for its own, much
// shorter-lived cache.
type domainHealthThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time // rule ID -> last real check time
}

func newDomainHealthThrottle() *domainHealthThrottle {
	return &domainHealthThrottle{last: make(map[string]time.Time)}
}

// ready reports whether ruleID is due for a real check, recording now as
// its last-checked time when it is. A rule never seen before is always
// ready, so a fresh control plane restart doesn't wait out a full
// interval before its first real check.
func (t *domainHealthThrottle) ready(ruleID string, interval time.Duration, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.last[ruleID]; ok && now.Sub(last) < interval {
		return false
	}
	t.last[ruleID] = now
	return true
}

// domainHealthAppName extracts the app name a domain_health rule watches
// from its own ResourceID ("service:" + name, resourceIDForApp in
// internal/api/metrics.go), the same reverse-parse resolveResourceID
// (crashloop.go) already does for a container's owning service.
func domainHealthAppName(resourceID string) (name string, ok bool) {
	return strings.CutPrefix(resourceID, "service:")
}

// EvaluateDomainHealth runs one KindDomainHealth rule against every domain
// currently configured on the app it's scoped to (ResourceID) and returns
// its updated evaluation state plus, when firing, one human-readable
// notice line per unhealthy domain, for Engine to attach to the outgoing
// Event.
//
// Unlike EvaluateCertExpiry/EvaluatePatchStatus/EvaluateNodeDiskSpace
// (platform-wide, ResourceID only a display label), a domain_health
// rule's ResourceID is real: it picks out exactly which app's own domains
// this rule checks, the same app-scoped shape EvaluateThreshold/
// EvaluateCrashloop use. An app with no domains configured, or one that
// has since been deleted, is treated like "no recent data": neither
// confirms nor denies.
//
// r.ForDuration optionally debounces a single unhealthy check the same
// way EvaluateThreshold's own ForDuration does, since a real DNS lookup
// can blip; zero (the default) fires the instant any domain is unhealthy,
// matching every other non-threshold kind's own default.
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
		// The app this rule watches was deleted: nothing left to alert
		// on, so the rule simply goes quiet rather than erroring on
		// every future tick, the same stance
		// EvaluateScheduledTaskFailure takes on a deleted task.
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
