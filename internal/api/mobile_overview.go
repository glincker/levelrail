package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	mobileOverviewAppCap       = 50
	mobileOverviewAttentionTop = 5
	mobileOverviewIncidentScan = 100
)

type latestDeploymentStore interface {
	LatestDeploymentsFor(ctx context.Context, apps []string) (map[string]store.LatestDeploymentStatus, error)
}

type mobileOverviewServer struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Version string `json:"version"`
}

type mobileOverviewAttention struct {
	Count int              `json:"count"`
	Top   []attention.Item `json:"top"`
}

type mobileOverviewApp struct {
	Name             string           `json:"name"`
	Status           appStatusSummary `json:"status"`
	URL              string           `json:"url,omitempty"`
	LastDeployStatus string           `json:"last_deploy_status,omitempty"`
	LastDeployAt     *time.Time       `json:"last_deploy_at,omitempty"`
}

type mobileOverviewNodes struct {
	Online int `json:"online"`
	Total  int `json:"total"`
}

type mobileOverviewResponse struct {
	Server        mobileOverviewServer    `json:"server"`
	Attention     mobileOverviewAttention `json:"attention"`
	Apps          []mobileOverviewApp     `json:"apps"`
	AppsTotal     int                     `json:"apps_total"`
	Nodes         mobileOverviewNodes     `json:"nodes"`
	Deploys24h    int                     `json:"deploys_24h"`
	OpenIncidents int                     `json:"open_incidents"`
}

// handleMobileOverview serves GET /api/v1/mobile/overview: one phone sized
// round trip, with a content ETag so a refresh with no change costs a 304.
func (rt *Router) handleMobileOverview(w http.ResponseWriter, r *http.Request) {
	abilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.internalError(w, "api: mobile overview: resolve caller failed", err)
		return
	}
	now := time.Now()
	out := mobileOverviewResponse{Server: mobileOverviewServer{Name: rt.brand.Name, URL: rt.mobileServerURL(r), Version: version.Version}}
	out.Attention = topAttention(rt.attentionFeedItems(r, abilities, now))
	if out.Apps, out.AppsTotal, err = rt.mobileOverviewApps(r); err != nil {
		rt.internalError(w, "api: mobile overview: apps failed", err)
		return
	}
	if out.Deploys24h, err = rt.mobileDeploys24h(r, now); err != nil {
		rt.internalError(w, "api: mobile overview: deploys failed", err)
		return
	}
	out.Nodes = rt.mobileNodeCounts(r.Context())
	out.OpenIncidents = rt.mobileOpenIncidents(r.Context(), now)

	body, err := json.Marshal(out)
	if err != nil {
		rt.internalError(w, "api: mobile overview: encode failed", err)
		return
	}
	sum := sha256.Sum256(body)
	etag := `W/"` + hex.EncodeToString(sum[:12]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimSpace(part)
		if p == "*" || p == etag || "W/"+p == etag {
			return true
		}
	}
	return false
}

func topAttention(items []attention.Item) mobileOverviewAttention {
	rank := map[string]int{attention.Critical: 0, attention.Warning: 1, attention.Info: 2}
	sorted := slices.Clone(items)
	slices.SortStableFunc(sorted, func(a, b attention.Item) int { return rank[a.Severity] - rank[b.Severity] })
	return mobileOverviewAttention{Count: len(items), Top: sorted[:min(len(sorted), mobileOverviewAttentionTop)]}
}

func (rt *Router) mobileOverviewApps(r *http.Request) ([]mobileOverviewApp, int, error) {
	fs, ok := rt.apps.(appFilterStore)
	if !ok {
		return nil, 0, errNoFilteredList
	}
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		return nil, 0, err
	}
	f := store.AppListFilter{Limit: mobileOverviewAppCap}
	if filtered {
		f.Limit = 0
	}
	svcs, total, err := fs.ListDesiredServicesFiltered(r.Context(), f)
	if err != nil {
		return nil, 0, err
	}
	if filtered {
		visible := svcs[:0:0]
		for _, s := range svcs {
			if canRead(s.Name) {
				visible = append(visible, s)
			}
		}
		total, svcs = len(visible), pageSlice(visible, mobileOverviewAppCap, 0)
	}
	names := make([]string, len(svcs))
	controllers := make([]string, len(svcs))
	for i, s := range svcs {
		names[i], controllers[i] = s.Name, applicationControllerName(s.Name)
	}
	conds, err := rt.deploys.GetConditionsForControllers(r.Context(), controllers)
	if err != nil {
		return nil, 0, err
	}
	latest := map[string]store.LatestDeploymentStatus{}
	if ls, ok := rt.deployAttempts.(latestDeploymentStore); ok {
		if latest, err = ls.LatestDeploymentsFor(r.Context(), names); err != nil {
			return nil, 0, err
		}
	}
	out := make([]mobileOverviewApp, 0, len(svcs))
	for _, s := range svcs {
		a := mobileOverviewApp{Name: s.Name, Status: summarizeAppConditions(conds[applicationControllerName(s.Name)])}
		if len(s.Domains) > 0 {
			a.URL = "https://" + s.Domains[0]
		}
		if l, ok := latest[s.Name]; ok {
			at := l.StartedAt
			if l.FinishedAt != nil {
				at = *l.FinishedAt
			}
			a.LastDeployStatus, a.LastDeployAt = l.Status, &at
		}
		out = append(out, a)
	}
	return out, total, nil
}

func (rt *Router) mobileDeploys24h(r *http.Request, now time.Time) (int, error) {
	ds, ok := rt.deployAttempts.(deploymentStore)
	if !ok {
		return 0, nil
	}
	visible, err := rt.visibleAppNames(r)
	if err != nil {
		return 0, err
	}
	since := now.Add(-24 * time.Hour)
	briefs, err := ds.ListDeploymentBriefs(r.Context(), since, visible)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range briefs {
		if !b.StartedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (rt *Router) mobileNodeCounts(ctx context.Context) mobileOverviewNodes {
	if rt.nodes == nil {
		return mobileOverviewNodes{}
	}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.logger.Warn("api: mobile overview: list nodes failed")
		return mobileOverviewNodes{}
	}
	out := mobileOverviewNodes{Total: len(nodes)}
	for _, n := range nodes {
		if n.Status == store.NodeStatusOnline {
			out.Online++
		}
	}
	return out
}

func (rt *Router) mobileOpenIncidents(ctx context.Context, now time.Time) int {
	if rt.statusPage == nil {
		return 0
	}
	incidents, err := rt.statusPage.ListPublicStatusIncidents(ctx, mobileOverviewIncidentScan)
	if err != nil {
		rt.logger.Warn("api: mobile overview: list incidents failed")
		return 0
	}
	n := 0
	for _, i := range incidents {
		if i.Kind == statuspage.IncidentKind && i.ResolvedAt == nil && !i.StartsAt.After(now) {
			n++
		}
	}
	return n
}
