package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// visitorWindows maps the accepted window names to their length.
var visitorWindows = map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// VisitorSource summarizes edge traffic for an app without exposing any client detail.
type VisitorSource interface {
	AppVisitors(ctx context.Context, app string, o ingress.VisitorSummaryOptions) ingress.VisitorSummary
}

// AppVisitors implements VisitorSource.
func (e EdgeAccess) AppVisitors(ctx context.Context, app string, o ingress.VisitorSummaryOptions) ingress.VisitorSummary {
	return e.Log.SummarizeWithHistory(ctx, app, e.Stats.KeysForApp(app), o)
}

// AppVisitorsResponse is the body of GET /api/v1/apps/{name}/visitors.
type AppVisitorsResponse struct {
	App         string    `json:"app"`
	Domain      string    `json:"domain,omitempty"`
	Window      string    `json:"window"`
	Since       time.Time `json:"since"`
	GeneratedAt time.Time `json:"generated_at"`
	ingress.VisitorSummary
}

// handleAppVisitors returns the privacy-safe visitor summary for an app. Windows of 1h, 24h, 7d and 30d
// merge stored hourly rollups with the live ring, so history survives a restart while the retention setting allows it.
func (rt *Router) handleAppVisitors(w http.ResponseWriter, r *http.Request) {
	res, ok := rt.visitorSummary(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

// visitorSummary validates the query and computes the summary shared by the JSON and CSV endpoints.
// It writes the error response itself and reports false when the request cannot be served.
func (rt *Router) visitorSummary(w http.ResponseWriter, r *http.Request) (AppVisitorsResponse, bool) {
	src, ok := rt.access.(VisitorSource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return AppVisitorsResponse{}, false
	}
	q := r.URL.Query()
	window := q.Get("window")
	if window == "" {
		window = "24h"
	}
	span, ok := visitorWindows[window]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 1h, 24h, 7d or 30d")
		return AppVisitorsResponse{}, false
	}
	now := time.Now().UTC()
	domain := strings.TrimSpace(q.Get("domain"))
	sum := src.AppVisitors(r.Context(), r.PathValue("name"), ingress.VisitorSummaryOptions{Domain: domain, Since: now.Add(-span), Until: now})
	return AppVisitorsResponse{
		App: r.PathValue("name"), Domain: domain, Window: window, Since: now.Add(-span), GeneratedAt: now, VisitorSummary: sum,
	}, true
}
