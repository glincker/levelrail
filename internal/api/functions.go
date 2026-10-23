package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

var functionWindows = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

type functionResource struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	Port        int    `json:"port"`
	IdleMinutes int    `json:"idle_minutes"`
	Sleeping    bool   `json:"sleeping"`
	URL         string `json:"url,omitempty"`
	// Invocations, ErrorRate5xx and P95Ms cover the requested window; absent when nothing was called.
	Invocations  float64 `json:"invocations"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	P95Ms        float64 `json:"p95_ms"`
}

type functionsResponse struct {
	WindowSeconds float64            `json:"window_seconds"`
	Functions     []functionResource `json:"functions"`
}

// handleListFunctions handles GET /api/v1/functions: apps that sleep when idle
// and hold the request that wakes them, with call count and p95 duration.
func (rt *Router) handleListFunctions(w http.ResponseWriter, r *http.Request) {
	window := functionWindows["24h"]
	if v := r.URL.Query().Get("window"); v != "" {
		d, ok := functionWindows[v]
		if !ok {
			writeError(w, http.StatusBadRequest, "window must be 1h, 24h or 7d")
			return
		}
		window = d
	}
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: list functions: resolve caller failed", err)
		return
	}
	sleeps, err := rt.appSleep.ListAppSleep(r.Context())
	if err != nil {
		rt.internalError(w, "api: list functions: load sleep settings failed", err)
		return
	}
	out := functionsResponse{WindowSeconds: window.Seconds(), Functions: []functionResource{}}
	now := time.Now()
	for _, s := range sleeps {
		if !s.HoldRequests || s.IdleMinutes <= 0 || (filtered && !canRead(s.ServiceName)) {
			continue
		}
		svc, err := rt.apps.GetDesiredService(r.Context(), s.ServiceName)
		if err != nil {
			continue
		}
		fn := functionResource{Name: svc.Name, Image: svc.Image, Port: svc.Port, IdleMinutes: s.IdleMinutes, Sleeping: s.Sleeping}
		if len(svc.Domains) > 0 {
			fn.URL = "https://" + svc.Domains[0]
		} else {
			fn.URL = rt.fallbackURLFor(r.Context(), *svc)
		}
		if rt.telemetry != nil {
			if sum, err := telemetry.SummarizeRequests(r.Context(), rt.telemetry, svc.Name, window, now); err == nil && sum.HasTraffic {
				fn.Invocations, fn.ErrorRate5xx, fn.P95Ms = sum.Requests, sum.ErrorRate5xx, sum.P95Ms
			}
		}
		out.Functions = append(out.Functions, fn)
	}
	sort.Slice(out.Functions, func(i, j int) bool { return out.Functions[i].Name < out.Functions[j].Name })
	writeJSON(w, http.StatusOK, out)
}

// handleFunctionStarters handles GET /api/v1/functions/starters: the tested utility starters behind the idea presets.
func (rt *Router) handleFunctionStarters(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"starters": functions.Starters(), "port": functions.DefaultPort})
}

// handleFunctionRuntimes handles GET /api/v1/functions/runtimes: the starter handler for each language.
func (rt *Router) handleFunctionRuntimes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"runtimes": functions.Runtimes(), "port": functions.DefaultPort})
}
