package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// visitorWindows maps the accepted window names to their length.
var visitorWindows = map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// VisitorSource summarizes edge traffic for an app without exposing any client detail.
type VisitorSource interface {
	AppVisitors(app string, o ingress.VisitorSummaryOptions) ingress.VisitorSummary
}

// AppVisitors implements VisitorSource.
func (e EdgeAccess) AppVisitors(app string, o ingress.VisitorSummaryOptions) ingress.VisitorSummary {
	return e.Log.Summarize(e.Stats.KeysForApp(app), o)
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

func (rt *Router) handleAppVisitors(w http.ResponseWriter, r *http.Request) {
	src, ok := rt.access.(VisitorSource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	q := r.URL.Query()
	window := q.Get("window")
	if window == "" {
		window = "24h"
	}
	span, ok := visitorWindows[window]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 1h, 24h, 7d or 30d")
		return
	}
	now := time.Now().UTC()
	domain := strings.TrimSpace(q.Get("domain"))
	sum := src.AppVisitors(r.PathValue("name"), ingress.VisitorSummaryOptions{Domain: domain, Since: now.Add(-span), Until: now})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, AppVisitorsResponse{
		App: r.PathValue("name"), Domain: domain, Window: window, Since: now.Add(-span), GeneratedAt: now, VisitorSummary: sum,
	})
}
