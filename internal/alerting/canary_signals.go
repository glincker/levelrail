package alerting

import (
	"context"
	"strings"
)

// CanarySignals feeds a staged canary's rollback decision from liveness failures and one alert rule.
type CanarySignals struct {
	health AppHealthSource
	rules  interface {
		ListEnabledRules(ctx context.Context) ([]Rule, error)
	}
}

// NewCanarySignals builds the source; either argument may be nil to disable that signal.
func NewCanarySignals(health AppHealthSource, rules interface {
	ListEnabledRules(ctx context.Context) ([]Rule, error)
}) *CanarySignals {
	return &CanarySignals{health: health, rules: rules}
}

// HealthFailures is the largest failure count among the service's unhealthy containers.
func (c *CanarySignals) HealthFailures(service string) uint64 {
	if c == nil || c.health == nil {
		return 0
	}
	var most int
	for _, a := range c.health.UnhealthyApps() {
		if a.Service == service && a.Failures > most {
			most = a.Failures
		}
	}
	return uint64(most)
}

// AlertFiring reports whether the enabled rule with this id is firing now.
func (c *CanarySignals) AlertFiring(ctx context.Context, ruleID string) bool {
	if c == nil || c.rules == nil || strings.TrimSpace(ruleID) == "" {
		return false
	}
	rules, err := c.rules.ListEnabledRules(ctx)
	if err != nil {
		return false
	}
	for _, r := range rules {
		if r.ID == ruleID {
			return r.Firing
		}
	}
	return false
}
