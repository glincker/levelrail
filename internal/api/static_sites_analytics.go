package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/ingress"
	"github.com/glincker/levelrail/internal/pages"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

// handleStaticSiteAnalytics handles GET /api/v1/static-sites/{name}/analytics.
// It only reshapes the edge visitor summary and access ring: no IP, cookie or query string.
func (rt *Router) handleStaticSiteAnalytics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "static site not found"), staticSiteRef(name), appRef(name)) {
		return
	}
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}
	span, ok := pages.Ranges[rng]
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be 24h, 7d or 30d")
		return
	}
	src, ok := rt.access.(VisitorSource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	days := 30
	if rt.runtimeSettings != nil {
		days = rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyPagesAnalyticsRetentionDays)
	}
	if !pages.RangeFits(span, days) {
		writeError(w, http.StatusBadRequest, "range exceeds the configured analytics retention")
		return
	}
	now := time.Now().UTC()
	since := now.Add(-span)
	topN := envInt("APP_PAGES_TRAFFIC_TOP_N", maxTrafficTopPaths)
	sum := src.AppVisitors(r.Context(), name, ingress.VisitorSummaryOptions{Since: since, Until: now, TopN: envInt("APP_PAGES_TRAFFIC_SCAN_N", 500)})
	paths := make([]ingress.CountedItem, len(sum.TopPaths))
	for i, p := range sum.TopPaths {
		paths[i] = ingress.CountedItem{Label: stripQuery(p.Label), Count: p.Count}
	}
	refs := make([]pages.Count, len(sum.TopRefs))
	for i, c := range sum.TopRefs {
		refs[i] = pages.Count{Label: c.Label, Count: c.Count}
	}
	out := pages.Analytics{
		Site: name, Range: rng, Since: since, GeneratedAt: now, RetentionDays: days, Requests: sum.Requests,
		TopPages: []pages.Count{}, TopReferrers: pages.MergeReferrers(refs, topN), Status: []pages.StatusCount{},
	}
	for _, p := range topPages(paths, topN) {
		out.TopPages = append(out.TopPages, pages.Count{Label: p.Label, Count: p.Count})
	}
	if rt.access != nil {
		recs := rt.access.AppAccess(name, ingress.AccessFilter{Since: since, Limit: 500})
		codes := make([]int, 0, len(recs))
		for _, rec := range recs {
			codes = append(codes, rec.Status)
		}
		out.Status = pages.StatusBreakdown(codes)
		out.Partial = len(recs) < sum.Requests
	} else {
		out.Partial = true
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// stripQuery drops any query or fragment so a stored path never carries one.
func stripQuery(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		return p[:i]
	}
	return p
}
