package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/glincker/levelrail/internal/ingress"
)

const maxTrafficTopPaths = 10

// pagesTrafficWindows are the windows the Pages traffic endpoint accepts.
var pagesTrafficWindows = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// TrafficSeriesSource buckets edge traffic for an app over time.
type TrafficSeriesSource interface {
	AppTrafficSeries(ctx context.Context, app string, since, until time.Time, buckets int) []ingress.TrafficPoint
}

// AppTrafficSeries implements TrafficSeriesSource.
func (e EdgeAccess) AppTrafficSeries(ctx context.Context, app string, since, until time.Time, buckets int) []ingress.TrafficPoint {
	return e.Log.TrafficSeries(ctx, app, e.Stats.KeysForApp(app), since, until, buckets)
}

// pagesTrafficBuckets is the series length, clamped so a bad env value cannot blow up the response.
func pagesTrafficBuckets() int {
	return min(max(envInt("APP_PAGES_TRAFFIC_BUCKETS", 24), 2), 120)
}

// pagesTrafficRetention caps the longest range the series endpoint serves.
func pagesTrafficRetention() time.Duration {
	return time.Duration(envInt("APP_PAGES_TRAFFIC_RETENTION_DAYS", 30)) * 24 * time.Hour
}

type trafficAssets struct {
	Requests int   `json:"requests"`
	Bytes    int64 `json:"bytes"`
}

// staticSiteTrafficResponse is the body of GET /api/v1/static-sites/{name}/traffic.
// Partial is true when bandwidth or the 404 list covers fewer requests than the
// request total, because those come from the in-memory access ring.
type staticSiteTrafficResponse struct {
	Name         string                 `json:"name"`
	Window       string                 `json:"window"`
	Since        time.Time              `json:"since"`
	GeneratedAt  time.Time              `json:"generated_at"`
	Requests     int                    `json:"requests"`
	Visitors     int                    `json:"visitors"`
	BytesOut     int64                  `json:"bytes_out"`
	Status       map[string]int         `json:"status"`
	TopPaths     []ingress.CountedItem  `json:"top_paths"`
	Top404Paths  []ingress.CountedItem  `json:"top_404_paths"`
	TopPages     []ingress.CountedItem  `json:"top_pages"`
	Assets       trafficAssets          `json:"assets"`
	BrokenLinks  []ingress.CountedItem  `json:"broken_links"`
	Probes       []ingress.CountedItem  `json:"probe_requests"`
	Series       []ingress.TrafficPoint `json:"series"`
	Partial      bool                   `json:"partial"`
	PartialNotes []string               `json:"partial_notes,omitempty"`
}

// handleStaticSiteTraffic handles GET /api/v1/static-sites/{name}/traffic. It
// only reads the existing visitor summary and access ring, and collects nothing new.
func (rt *Router) handleStaticSiteTraffic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "static site not found"), staticSiteRef(name), appRef(name)) {
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = r.URL.Query().Get("range")
	}
	if window == "" {
		window = "24h"
	}
	span, ok := pagesTrafficWindows[window]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 24h, 7d or 30d")
		return
	}
	src, ok := rt.access.(VisitorSource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	if span > pagesTrafficRetention() {
		writeError(w, http.StatusBadRequest, "window exceeds the configured traffic retention")
		return
	}
	now := time.Now().UTC()
	since := now.Add(-span)
	sum := src.AppVisitors(r.Context(), name, ingress.VisitorSummaryOptions{Since: since, Until: now, TopN: envInt("APP_PAGES_TRAFFIC_SCAN_N", 500)})
	out := staticSiteTrafficResponse{
		Name: name, Window: window, Since: since, GeneratedAt: now,
		Requests: sum.Requests, Visitors: sum.Visitors, Status: sum.Status,
		TopPaths: capItems(sum.TopPaths, maxTrafficTopPaths), Top404Paths: []ingress.CountedItem{},
		TopPages:    topPages(sum.TopPaths, envInt("APP_PAGES_TRAFFIC_TOP_N", maxTrafficTopPaths)),
		BrokenLinks: []ingress.CountedItem{}, Probes: []ingress.CountedItem{},
	}
	out.Series = []ingress.TrafficPoint{}
	if ss, ok := rt.access.(TrafficSeriesSource); ok {
		out.Series = ss.AppTrafficSeries(r.Context(), name, since, now, pagesTrafficBuckets())
	}
	if out.Status == nil {
		out.Status = map[string]int{}
	}
	if out.TopPaths == nil {
		out.TopPaths = []ingress.CountedItem{}
	}
	if rt.access != nil {
		recs := rt.access.AppAccess(name, ingress.AccessFilter{Since: since, Limit: 500})
		counts, broken, probes := map[string]int{}, map[string]int{}, map[string]int{}
		for _, rec := range recs {
			out.BytesOut += rec.BytesOut
			if kind, _, _ := classifyTrafficPath(rec.Path); kind == trafficPathAsset && !isProbePath(rec.Path) {
				out.Assets.Requests++
				out.Assets.Bytes += rec.BytesOut
			}
			if rec.Status == http.StatusNotFound {
				counts[rec.Path]++
				if isProbePath(rec.Path) {
					probes[rec.Path]++
				} else {
					broken[rec.Path]++
				}
			}
		}
		n := envInt("APP_PAGES_TRAFFIC_TOP_N", maxTrafficTopPaths)
		out.Top404Paths = topCounts(counts, maxTrafficTopPaths)
		out.BrokenLinks = topCounts(broken, n)
		out.Probes = topCounts(probes, n)
		if len(recs) < out.Requests {
			out.Partial = true
			out.PartialNotes = append(out.PartialNotes, "bandwidth, asset totals and 404 lists cover only the most recent requests held in memory")
		}
	} else {
		out.Partial = true
		out.PartialNotes = append(out.PartialNotes, "bandwidth, asset totals and 404 lists are unavailable")
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func topCounts(m map[string]int, n int) []ingress.CountedItem {
	out := make([]ingress.CountedItem, 0, len(m))
	for k, v := range m {
		out = append(out, ingress.CountedItem{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func capItems(in []ingress.CountedItem, n int) []ingress.CountedItem {
	if len(in) > n {
		return in[:n]
	}
	return in
}

// topPages merges page-like paths by normalised key and returns the top n.
func topPages(paths []ingress.CountedItem, n int) []ingress.CountedItem {
	counts, labels := map[string]int{}, map[string]string{}
	for _, p := range paths {
		kind, label, key := classifyTrafficPath(p.Label)
		if kind != trafficPathPage {
			continue
		}
		counts[key] += p.Count
		if _, ok := labels[key]; !ok {
			labels[key] = label
		}
	}
	out := make([]ingress.CountedItem, 0, len(counts))
	for k, c := range counts {
		out = append(out, ingress.CountedItem{Label: labels[k], Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return capItems(out, n)
}
