package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// invalidateAll drops the cached posture and the cached exposure audit, so
// the next read re-measures instead of replaying a result from before a fix.
func (c *postureCache) invalidateAll() {
	c.mu.Lock()
	c.at = time.Time{}
	c.exposureAt = time.Time{}
	c.mu.Unlock()
}

func (rt *Router) unshieldedDomains(ctx context.Context, domains []string) []string {
	if rt.domainPolicy.store == nil {
		return nil
	}
	var out []string
	for _, d := range domains {
		row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(ctx, d, trafficpolicy.KindShield)
		if err != nil {
			rt.logger.Warn("api: reachability: read shield failed", slog.String("domain", d), slog.String("error", err.Error()))
			continue
		}
		var sh trafficpolicy.Shield
		if found && json.Unmarshal(row.Spec, &sh) == nil && sh.Enabled {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (rt *Router) reachabilityReport(ctx context.Context) (exposure.ReachReport, error) {
	rep, err := rt.exposureReport(ctx, "", false)
	if err != nil {
		return exposure.ReachReport{}, err
	}
	in := exposure.ReachInput{Now: rep.GeneratedAt}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return exposure.ReachReport{}, err
	}
	for _, svc := range services {
		in.Apps = append(in.Apps, exposure.ReachApp{Name: svc.Name, Domains: svc.Domains, Unshielded: rt.unshieldedDomains(ctx, svc.Domains)})
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return exposure.ReachReport{}, err
	}
	for _, d := range dbs {
		if d.PubliclyAccessible {
			in.Databases = append(in.Databases, exposure.ReachDB{Name: d.Name, PublicPort: d.PublicPort})
		}
	}
	for _, n := range rep.Nodes {
		rn := exposure.ReachNode{Name: n.NodeName, Local: n.Local, RulesReadable: n.RulesReadable}
		for _, f := range n.Findings {
			rn.Findings = append(rn.Findings, f.Finding)
		}
		in.Nodes = append(in.Nodes, rn)
	}
	return exposure.BuildReach(in), nil
}

// handleSecurityReachability answers "what can the internet reach" for every
// app and database. refresh=true is the Re-check button: it drops the cached
// audit so the security score is measured again too.
func (rt *Router) handleSecurityReachability(w http.ResponseWriter, r *http.Request) {
	if rt.exposure == nil || rt.apps == nil || rt.databases == nil {
		writeError(w, http.StatusNotImplemented, "exposure audit is not configured on this control plane")
		return
	}
	if r.URL.Query().Get("refresh") == "true" {
		rt.sec.posture.invalidateAll()
	}
	rep, err := rt.reachabilityReport(r.Context())
	if err != nil {
		rt.internalError(w, "api: reachability report failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
