package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

const maxTrafficTopPaths = 10

// pagesTrafficWindows are the windows the Pages traffic endpoint accepts.
var pagesTrafficWindows = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}

// staticSiteTrafficResponse is the body of GET /api/v1/static-sites/{name}/traffic.
// Partial is true when bandwidth or the 404 list covers fewer requests than the
// request total, because those come from the in-memory access ring.
type staticSiteTrafficResponse struct {
	Name         string                `json:"name"`
	Window       string                `json:"window"`
	Since        time.Time             `json:"since"`
	GeneratedAt  time.Time             `json:"generated_at"`
	Requests     int                   `json:"requests"`
	Visitors     int                   `json:"visitors"`
	BytesOut     int64                 `json:"bytes_out"`
	Status       map[string]int        `json:"status"`
	TopPaths     []ingress.CountedItem `json:"top_paths"`
	Top404Paths  []ingress.CountedItem `json:"top_404_paths"`
	Partial      bool                  `json:"partial"`
	PartialNotes []string              `json:"partial_notes,omitempty"`
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
		window = "24h"
	}
	span, ok := pagesTrafficWindows[window]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 24h or 7d")
		return
	}
	src, ok := rt.access.(VisitorSource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	now := time.Now().UTC()
	since := now.Add(-span)
	sum := src.AppVisitors(r.Context(), name, ingress.VisitorSummaryOptions{Since: since, Until: now, TopN: maxTrafficTopPaths})
	out := staticSiteTrafficResponse{
		Name: name, Window: window, Since: since, GeneratedAt: now,
		Requests: sum.Requests, Visitors: sum.Visitors, Status: sum.Status,
		TopPaths: sum.TopPaths, Top404Paths: []ingress.CountedItem{},
	}
	if out.Status == nil {
		out.Status = map[string]int{}
	}
	if out.TopPaths == nil {
		out.TopPaths = []ingress.CountedItem{}
	}
	if rt.access != nil {
		recs := rt.access.AppAccess(name, ingress.AccessFilter{Since: since, Limit: 500})
		counts := map[string]int{}
		for _, rec := range recs {
			out.BytesOut += rec.BytesOut
			if rec.Status == http.StatusNotFound {
				counts[rec.Path]++
			}
		}
		out.Top404Paths = topCounts(counts, maxTrafficTopPaths)
		if len(recs) < out.Requests {
			out.Partial = true
			out.PartialNotes = append(out.PartialNotes, "bandwidth and top 404 paths cover only the most recent requests held in memory")
		}
	} else {
		out.Partial = true
		out.PartialNotes = append(out.PartialNotes, "bandwidth and top 404 paths are unavailable")
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
