package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cdn"
	"github.com/GLINCKER/levelrail/internal/ingress"
)

// registerCDNEdgeRoutes wires the per-app edge status and the guided setup.
// Setup can change DNS records, so it needs AbilityWriteSensitive.
func (rt *Router) registerCDNEdgeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/edge", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppEdge))
	mux.HandleFunc("POST /api/v1/apps/{name}/cdn/setup", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetupAppCDN))
}

// edgePurgeShown caps the purge records one edge response carries.
const edgePurgeShown = 10

type edgeCache struct {
	Hits    uint64  `json:"hits"`
	Misses  uint64  `json:"misses"`
	HitRate float64 `json:"hit_rate"`
	Entries int     `json:"entries"`
}

type edgeDeploy struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Source     string     `json:"source"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type edgeDNS struct {
	// Managed counts DNS records Levelrail created for the app.
	Managed int `json:"managed_records"`
}

// edgeResource is GET /api/v1/apps/{name}/edge: the deploy, DNS and CDN state
// of one app in a single answer, so a pipeline view needs no second call.
type edgeResource struct {
	App string `json:"app"`
	// Health is direct (no proxy), ok or attention (see Issues).
	Health string `json:"health"`
	// Proxied is the effective setting; Override is the app's own, null when inherited.
	Proxied    bool              `json:"proxied"`
	Override   *bool             `json:"override"`
	Provider   string            `json:"provider,omitempty"`
	Zone       string            `json:"zone,omitempty"`
	SSLMode    string            `json:"ssl_mode,omitempty"`
	SSLWarning string            `json:"ssl_warning,omitempty"`
	Domains    []string          `json:"domains"`
	Deploy     *edgeDeploy       `json:"deploy,omitempty"`
	DNS        edgeDNS           `json:"dns"`
	Cache      edgeCache         `json:"cache"`
	LastPurge  *cdn.PurgeRecord  `json:"last_purge,omitempty"`
	Purges     []cdn.PurgeRecord `json:"purges"`
	Issues     []string          `json:"issues"`
}

// handleGetAppEdge handles GET /api/v1/apps/{name}/edge.
func (rt *Router) handleGetAppEdge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	if rt.ingressSettings == nil {
		writeError(w, http.StatusNotImplemented, "edge status is not available")
		return
	}
	res, err := rt.edgeStatus(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: app edge: read failed", err, slog.String("name", name))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) edgeStatus(ctx context.Context, app string) (edgeResource, error) {
	cur, err := rt.appCDNResource(ctx, app)
	if err != nil {
		return edgeResource{}, err
	}
	res := edgeResource{App: app, Proxied: cur.Effective, Override: cur.Proxied, Domains: []string{}, Issues: []string{}, Health: "direct"}
	domains, err := rt.appServiceDomains(ctx, app)
	if err != nil {
		return res, err
	}
	res.Domains = domains
	rt.fillEdgeDeploy(ctx, app, &res)
	rt.fillEdgeCache(domains, &res)
	rt.fillEdgeDNS(ctx, domains, &res)
	res.Purges = cdn.DefaultHistory().List(app, edgePurgeShown)
	if len(res.Purges) > 0 {
		res.LastPurge = &res.Purges[0]
	}
	if len(domains) == 0 {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, cdnPurgeBudget())
	defer cancel()
	t, terr := rt.resolveDNSTarget(ctx, domains[0])
	if terr == nil && t != nil {
		res.Zone = strings.TrimSuffix(t.Zone, ".")
		if p, ok := t.Proxy.(cdnZoneClient); ok {
			res.Provider = cdnProviderName(p)
		}
	}
	if !res.Proxied {
		return res, nil
	}
	res.Health = "ok"
	switch {
	case terr != nil || res.Provider == "":
		res.Issues = append(res.Issues, "no connected CDN provider can manage this app's domains")
	default:
		res.SSLMode, res.SSLWarning = rt.cdnSSLCheck(ctx, app)
	}
	if res.SSLMode == "off" || res.SSLMode == "flexible" {
		res.Issues = append(res.Issues, res.SSLWarning)
	}
	if res.LastPurge != nil && !res.LastPurge.OK {
		res.Issues = append(res.Issues, "the last cache purge failed")
	}
	if len(res.Issues) > 0 {
		res.Health = "attention"
	}
	return res, nil
}

func (rt *Router) appServiceDomains(ctx context.Context, app string) ([]string, error) {
	out := []string{}
	if rt.domains == nil {
		return out, nil
	}
	all, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range all {
		if d.ServiceName == app {
			out = append(out, strings.TrimSuffix(d.Domain, "."))
		}
	}
	return out, nil
}

func (rt *Router) fillEdgeDeploy(ctx context.Context, app string, res *edgeResource) {
	if rt.deployAttempts == nil {
		return
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, app)
	if err != nil || len(attempts) == 0 {
		return
	}
	a := attempts[0]
	res.Deploy = &edgeDeploy{ID: a.ID, Status: a.Status, Source: a.Source, CommitSHA: a.CommitSHA, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt}
}

func (rt *Router) fillEdgeCache(domains []string, res *edgeResource) {
	var all []ingress.CacheStats
	for _, d := range domains {
		all = append(all, ingress.DefaultResponseCache().Stats(d))
	}
	m := ingress.MergeCacheStats(all...)
	res.Cache = edgeCache{Hits: m.Hits, Misses: m.Misses, HitRate: m.HitRate(), Entries: m.Entries}
}

func (rt *Router) fillEdgeDNS(ctx context.Context, domains []string, res *edgeResource) {
	ms := rt.managedDNS()
	if ms == nil {
		return
	}
	for _, d := range domains {
		rows, err := ms.ListManagedDNSRecords(ctx, d)
		if err != nil {
			continue
		}
		for _, row := range rows {
			if row.AppName == res.App && row.RecordType != "TXT" {
				res.DNS.Managed++
			}
		}
	}
}
