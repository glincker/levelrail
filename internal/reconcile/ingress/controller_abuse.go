package ingress

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Reasons an app's abuse defense reports, shown beside its policy.
const (
	AbuseReasonEnforcing      = "Enforcing"
	AbuseReasonLogOnly        = "LogOnly"
	AbuseReasonOff            = "Off"
	AbuseReasonNoRoutes       = "NoRoutes"
	AbuseReasonClientIPHidden = "ClientIPHidden"
	AbuseReasonInvalidPolicy  = "InvalidPolicy"
)

// AbusePolicyStore is optional on ServiceStore: a store without it routes
// every app with no abuse defense.
type AbusePolicyStore interface {
	ListAppAbusePolicies(ctx context.Context) ([]store.AppAbusePolicy, error)
}

// AbuseConditionStore is optional too; without it the condition is not kept.
type AbuseConditionStore interface {
	SetAppAbuseCondition(ctx context.Context, app, status, reason, message string) error
}

// abuseApplied remembers which apps had an active policy on the last pass, so
// a store failure can count the requests those apps now serve unchecked.
type abuseApplied struct {
	mu   sync.Mutex
	apps map[string]struct{}
}

func (a *abuseApplied) swap(next map[string]struct{}) map[string]struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.apps
	a.apps = next
	return prev
}

// applyAbusePolicies attaches each app's abuse defense to its proxy routes.
// It never fails the pass: when policies cannot be read the routes are served
// without the defense (fail open) and the apps that had one are counted.
func (c *Controller) applyAbusePolicies(ctx context.Context, routes []ingress.ProxyRoute, claimedHosts map[string]string, settings store.IngressSettings) []ingress.ProxyRoute {
	ps, ok := c.store.(AbusePolicyStore)
	if !ok {
		return routes
	}
	limits := abuse.LimitsFromEnv(os.Getenv)
	rows, err := ps.ListAppAbusePolicies(ctx)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: abuse policies unavailable, serving without them", slog.String("error", err.Error()))
		for app := range c.abuseApplied.swap(nil) {
			abuse.DefaultRegistry().NoteFailOpen(app, limits)
		}
		return routes
	}
	policies := make(map[string]abuse.Policy, len(rows))
	conds := make(map[string]abuseCondition, len(rows))
	for _, row := range rows {
		var p abuse.Policy
		if err := json.Unmarshal(row.Spec, &p); err == nil {
			p = p.Normalize(limits)
			err = p.Validate(limits)
			if err == nil {
				policies[row.App] = p
				continue
			}
		}
		c.logger.WarnContext(ctx, "ingress: skipping undecodable app abuse policy", slog.String("app", row.App))
		abuse.DefaultRegistry().NoteFailOpen(row.App, limits)
		conds[row.App] = abuseCondition{Status: reconcile.ConditionFalse, Reason: AbuseReasonInvalidPolicy, Message: "The saved policy is invalid, so it is not applied. Save it again."}
	}

	routed := map[string]bool{}
	active := map[string]struct{}{}
	for i := range routes {
		if len(routes[i].Hosts) == 0 {
			continue
		}
		app := claimedHosts[routes[i].Hosts[0]]
		p, ok := policies[app]
		if !ok || !p.Active() {
			continue
		}
		routes[i].Abuse = &ingress.AbusePolicy{App: app, Policy: p}
		routed[app] = true
		active[app] = struct{}{}
	}
	c.abuseApplied.swap(active)

	hidden := (settings.TLSTerminatedUpstream || settings.DNSProxied) && (c.hardening == nil || len(c.hardening.TrustedProxies) == 0)
	for app, p := range policies {
		conds[app] = abuseConditionFor(p, routed[app], hidden)
	}
	c.recordAbuseConditions(ctx, conds)
	return routes
}

type abuseCondition struct {
	Status  reconcile.ConditionStatus
	Reason  string
	Message string
}

func abuseConditionFor(p abuse.Policy, routed, hiddenClients bool) abuseCondition {
	switch {
	case !p.Active():
		return abuseCondition{Status: reconcile.ConditionFalse, Reason: AbuseReasonOff, Message: "The defense is off."}
	case !routed:
		return abuseCondition{Status: reconcile.ConditionFalse, Reason: AbuseReasonNoRoutes, Message: "The app has no proxied route to protect (it may be stopped, asleep or a static site)."}
	case hiddenClients && (p.RPS > 0 || len(p.DenyCIDRs) > 0):
		return abuseCondition{Status: reconcile.ConditionFalse, Reason: AbuseReasonClientIPHidden, Message: "A proxy in front hides client addresses. Set the trusted proxies, or every visitor shares one rate limit."}
	case p.Mode == abuse.ModeLogOnly:
		return abuseCondition{Status: reconcile.ConditionTrue, Reason: AbuseReasonLogOnly, Message: "Counting what would be blocked. Nothing is blocked yet."}
	}
	return abuseCondition{Status: reconcile.ConditionTrue, Reason: AbuseReasonEnforcing, Message: "Blocking requests that break the policy."}
}

func (c *Controller) recordAbuseConditions(ctx context.Context, conds map[string]abuseCondition) {
	cs, ok := c.store.(AbuseConditionStore)
	if !ok {
		return
	}
	for app, cond := range conds {
		if err := cs.SetAppAbuseCondition(ctx, app, string(cond.Status), cond.Reason, cond.Message); err != nil {
			c.logger.WarnContext(ctx, "ingress: record abuse condition failed", slog.String("app", app), slog.String("error", err.Error()))
		}
	}
}
