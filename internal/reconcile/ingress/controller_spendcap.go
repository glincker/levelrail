package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	spendCapPageTitle   = "Temporarily unavailable"
	spendCapPageMessage = "This app has reached its monthly usage limit and is paused."
	spendCapMaxRetry    = 24 * time.Hour
)

// SpendCapEnforcementLister is optional on ServiceStore: without it spend
// caps are not applied.
type SpendCapEnforcementLister interface {
	ListSpendCapEnforcements(ctx context.Context, period string) ([]spendcap.Enforcement, error)
}

// applySpendCaps folds each enforced app cap into this pass's per-domain
// maps: pause becomes a maintenance route, throttle a per-client rate limit
// no looser than one already set. It returns the maintenance settings map,
// which it allocates when the store had none.
func (c *Controller) applySpendCaps(ctx context.Context, services []store.DesiredService, envSets map[string]map[string][]string, fallbackOff bool, fallbacks ingress.FallbackResolver, now time.Time, maintenance map[string]bool, maintenanceSettings map[string]store.DomainMaintenance, waf map[string]store.DomainWAF) (map[string]store.DomainMaintenance, error) {
	l, ok := c.store.(SpendCapEnforcementLister)
	if !ok {
		return maintenanceSettings, nil
	}
	enforcements, err := l.ListSpendCapEnforcements(ctx, spendcap.Period(now))
	if err != nil {
		return maintenanceSettings, fmt.Errorf("list spend cap enforcements: %w", err)
	}
	if len(enforcements) == 0 {
		return maintenanceSettings, nil
	}
	byApp := make(map[string]spendcap.Enforcement, len(enforcements))
	for _, e := range enforcements {
		byApp[e.App] = e
	}
	retry := int(spendcap.NextPeriodStart(now).Sub(now) / time.Second)
	if limit := int(spendCapMaxRetry / time.Second); retry > limit {
		retry = limit
	}
	if maintenanceSettings == nil {
		maintenanceSettings = map[string]store.DomainMaintenance{}
	}
	for _, svc := range services {
		e, ok := byApp[svc.Name]
		if !ok {
			continue
		}
		hosts := routedDomains(svc, envSets)
		if len(hosts) == 0 && !fallbackOff {
			if h, ok := fallbacks.Domain(svc.Name); ok {
				hosts = []string{h}
			}
		}
		for _, host := range hosts {
			switch e.Action {
			case spendcap.ActionPause:
				if _, set := maintenanceSettings[host]; !set {
					maintenanceSettings[host] = store.DomainMaintenance{Domain: host, Title: spendCapPageTitle, Message: spendCapPageMessage, RetryAfterSeconds: retry}
				}
				maintenance[host] = true
			case spendcap.ActionThrottle:
				row := waf[host]
				row.Domain = host
				if row.RateLimitRPS <= 0 || e.ThrottleRPS < row.RateLimitRPS {
					row.RateLimitRPS = e.ThrottleRPS
					row.RateLimitBurst = 0
				}
				waf[host] = row
			}
		}
	}
	return maintenanceSettings, nil
}
