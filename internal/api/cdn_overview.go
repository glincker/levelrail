package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Trailing minutes the in-memory cache counters cover.
const cdnCacheCoverageMinutes = 30

// cdnSeriesPoints is how many buckets the overview chart is cut into.
const cdnSeriesPoints = 24

type cdnPoint struct {
	Time     time.Time `json:"time"`
	Requests float64   `json:"requests"`
	Errors   float64   `json:"errors_5xx"`
}

type cdnNode struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Region   string  `json:"region,omitempty"`
	Requests float64 `json:"requests"`
	Share    float64 `json:"share"`
	Apps     int     `json:"apps"`
}

type cdnCache struct {
	CoveredMinutes int                   `json:"covered_minutes"`
	Hits           uint64                `json:"hits"`
	Misses         uint64                `json:"misses"`
	HitRate        float64               `json:"hit_rate"`
	HitBytes       uint64                `json:"hit_bytes"`
	OriginBytes    uint64                `json:"origin_bytes"`
	Series         []ingress.CacheBucket `json:"series"`
}

type cdnOverviewResponse struct {
	Window         string     `json:"window"`
	From           time.Time  `json:"from"`
	To             time.Time  `json:"to"`
	HasTraffic     bool       `json:"has_traffic"`
	Requests       float64    `json:"requests"`
	RatePerSec     float64    `json:"rate_per_sec"`
	ErrorRate5xx   float64    `json:"error_rate_5xx"`
	BytesOut       float64    `json:"bytes_out"`
	Series         []cdnPoint `json:"series"`
	Cache          cdnCache   `json:"cache"`
	Nodes          []cdnNode  `json:"nodes"`
	RegionsLabeled bool       `json:"regions_labeled"`
}

// handleCDNOverview handles GET /api/v1/cdn/overview?window=5m|1h|24h: fleet
// requests, 5xx rate and bytes from ingress request metrics, the edge cache
// counters (last 30 minutes, since process start at most) and requests
// attributed to the node each app runs on. Visitor geography is not measured.
func (rt *Router) handleCDNOverview(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	label := r.URL.Query().Get("window")
	if label == "" {
		label = "1h"
	}
	window, ok := trafficWindows[label]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 5m, 1h or 24h")
		return
	}
	ctx := r.Context()
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.internalError(w, "api: cdn overview: list apps", err)
		return
	}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.internalError(w, "api: cdn overview: list nodes", err)
		return
	}
	to := time.Now().UTC()
	from := to.Add(-window)
	step := max(window/cdnSeriesPoints, telemetry.MinRequestStep)
	buckets := int(window/step) + 1
	resp := cdnOverviewResponse{Window: label, From: from, To: to, Series: make([]cdnPoint, buckets), Nodes: []cdnNode{}}
	for i := range resp.Series {
		resp.Series[i].Time = from.Add(time.Duration(i) * step)
	}

	perNode := map[string]*cdnNode{}
	var errs float64
	for _, svc := range svcs {
		points, qerr := telemetry.QueryRequests(ctx, rt.telemetry, svc.Name, from, to, step)
		if qerr != nil && points == nil {
			continue
		}
		var appReqs float64
		for _, p := range points {
			i := int(p.Timestamp.Sub(from) / step)
			if i < 0 || i >= buckets {
				continue
			}
			resp.Series[i].Requests += p.Requests
			resp.Series[i].Errors += p.Requests * p.ErrorRate5xx
			appReqs += p.Requests
			errs += p.Requests * p.ErrorRate5xx
		}
		resp.Requests += appReqs
		resp.BytesOut += sumMetric(ctx, rt.telemetry, svc.Name, telemetry.MetricHTTPBytesOut, from, to)
		if appReqs == 0 {
			continue
		}
		n := perNode[svc.NodeID]
		if n == nil {
			n = &cdnNode{ID: svc.NodeID}
			perNode[svc.NodeID] = n
		}
		n.Requests += appReqs
		n.Apps++
	}
	resp.HasTraffic = resp.Requests > 0
	resp.RatePerSec = resp.Requests / window.Seconds()
	resp.ErrorRate5xx = ratio(errs, resp.Requests)
	resp.Nodes, resp.RegionsLabeled = rankNodes(perNode, nodes, resp.Requests)

	all := ingress.DefaultResponseCache()
	var stats []ingress.CacheStats
	for _, svc := range svcs {
		for _, d := range svc.Domains {
			stats = append(stats, all.Stats(d))
		}
	}
	merged := ingress.MergeCacheStats(stats...)
	covered := min(int(window/time.Minute), cdnCacheCoverageMinutes)
	covered = max(covered, 1)
	hits, misses, hitBytes, missBytes := merged.Tail(covered)
	resp.Cache = cdnCache{CoveredMinutes: covered, Hits: hits, Misses: misses, HitBytes: hitBytes, OriginBytes: missBytes, Series: merged.Series}
	resp.Cache.HitRate = ratio(float64(hits), float64(hits+misses))
	if resp.Cache.Series == nil {
		resp.Cache.Series = []ingress.CacheBucket{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// rankNodes names each node's share. An app with no node id runs on the
// control plane's own machine.
func rankNodes(perNode map[string]*cdnNode, nodes []store.Node, total float64) ([]cdnNode, bool) {
	byID := map[string]store.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	out := make([]cdnNode, 0, len(perNode))
	labeled := false
	for id, n := range perNode {
		if node, ok := byID[id]; ok {
			n.Name, n.Region = node.Name, node.Region
		}
		if n.Name == "" && id != "" {
			n.Name = id
		}
		labeled = labeled || n.Region != ""
		n.Share = ratio(n.Requests, total)
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].ID < out[j].ID
	})
	return out, labeled
}
