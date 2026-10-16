package ingress

import (
	"context"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainMaintenanceSettingsLister is optional on ServiceStore: with it the
// page copy, Retry-After and bypass rules apply, without it the default page.
type DomainMaintenanceSettingsLister interface {
	ListActiveDomainMaintenance(ctx context.Context, now time.Time) ([]store.DomainMaintenance, error)
}

// domainMaintenanceSettings maps each domain in maintenance to its settings.
func (c *Controller) domainMaintenanceSettings(ctx context.Context, now time.Time) (map[string]store.DomainMaintenance, error) {
	l, ok := c.store.(DomainMaintenanceSettingsLister)
	if !ok {
		return nil, nil
	}
	rows, err := l.ListActiveDomainMaintenance(ctx, now)
	if err != nil {
		return nil, err
	}
	out := make(map[string]store.DomainMaintenance, len(rows))
	for _, m := range rows {
		out[m.Domain] = m
	}
	return out, nil
}

func maintenancePage(m store.DomainMaintenance) ingress.MaintenancePage {
	return ingress.MaintenancePage{Title: m.Title, Message: m.Message, RetryAfterSeconds: m.RetryAfterSeconds}
}

// maintenancePages is the plain maintenance route for each host: no backend
// needed, so it always holds.
func maintenancePages(hosts []string, settings map[string]store.DomainMaintenance) []ingress.MaintenanceRoute {
	out := make([]ingress.MaintenanceRoute, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, ingress.MaintenanceRoute{Hosts: []string{h}, Page: maintenancePage(settings[h])})
	}
	return out
}

// readinessPath is the app's HTTP readiness path, if it has one.
func readinessPath(svc store.DesiredService) string {
	if svc.Health == nil || svc.Health.Readiness == nil {
		return ""
	}
	r := svc.Health.Readiness
	if r.TCP || len(r.Exec) > 0 {
		return ""
	}
	p := strings.TrimSpace(r.Path)
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "*{}") {
		return ""
	}
	return p
}

// maintenanceGate returns the bypass gate for host, or nil when nothing may
// bypass it and the plain page is enough.
func maintenanceGate(svc store.DesiredService, m store.DomainMaintenance) *ingress.MaintenanceGate {
	g := &ingress.MaintenanceGate{Page: maintenancePage(m), BypassCIDRs: m.BypassCIDRs}
	if m.BypassHealthCheck {
		// "/" is the app's homepage, not a probe: bypassing it serves the real
		// document while its assets still get the maintenance response.
		if p := readinessPath(svc); p != "" && p != "/" {
			g.BypassPaths = []string{p}
		}
	}
	if len(g.BypassCIDRs) == 0 && len(g.BypassPaths) == 0 {
		return nil
	}
	return g
}

// splitMaintenanceBypass separates maintenance hosts with bypass rules,
// which need a real route behind the gate, from the plain ones.
func splitMaintenanceBypass(svc store.DesiredService, hosts []string, settings map[string]store.DomainMaintenance) (plain, gated []string) {
	for _, h := range hosts {
		if maintenanceGate(svc, settings[h]) != nil {
			gated = append(gated, h)
		} else {
			plain = append(plain, h)
		}
	}
	return plain, gated
}

// gatedMaintenanceRoutes builds one gated route per host. A host whose full
// route cannot be built (for example an unresolvable basic auth secret) is
// returned in fallback and must get the plain page, never an open route.
func (c *Controller) gatedMaintenanceRoutes(ctx context.Context, svc store.DesiredService, hosts []string, dial string, settings map[string]store.DomainMaintenance, f domainFeatures) (routes []ingress.ProxyRoute, fallback []string) {
	for _, h := range hosts {
		gate := maintenanceGate(svc, settings[h])
		rs := c.routesForService(ctx, []string{h}, dial, f.auth, f.waf, f.errorPages, f.hidden)
		if gate == nil || len(rs) != 1 {
			fallback = append(fallback, h)
			continue
		}
		rs[0].Maintenance = gate
		routes = append(routes, rs[0])
	}
	return routes, fallback
}
