package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// handleSLOPreview handles GET /api/v1/apps/{name}/slo-preview: the error
// budget left and the current burn rates for a candidate SLO
// (?objective=availability|latency, ?target=99.9, ?latency_ms=300), computed
// from the app's ingress request metrics exactly as an slo_burn rule would.
func (rt *Router) handleSLOPreview(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: slo preview: load app failed", err, slog.String("name", name))
		return
	}

	q := r.URL.Query()
	cfg := alerting.SLOConfig{Objective: q.Get("objective"), Target: 99.9}
	if cfg.Objective == "" {
		cfg.Objective = alerting.SLOAvailability
	}
	if v := q.Get("target"); v != "" {
		t, err := strconv.ParseFloat(v, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "target must be a number such as 99.9")
			return
		}
		cfg.Target = t
	}
	if v := q.Get("latency_ms"); v != "" {
		ms, err := strconv.ParseFloat(v, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "latency_ms must be a number")
			return
		}
		cfg.LatencyMs = ms
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	st, err := alerting.ComputeSLOStatus(r.Context(), rt.telemetry, name, cfg, alerting.SLOPolicyFromEnv(), time.Now())
	if err != nil {
		rt.internalError(w, "api: slo preview failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toSLOPreview(st))
}

type sloWindowResource struct {
	WindowSeconds int64   `json:"window_seconds"`
	Requests      float64 `json:"requests"`
	Bad           float64 `json:"bad"`
	BurnRate      float64 `json:"burn_rate"`
}

type sloTierResource struct {
	Name          string  `json:"name"`
	Factor        float64 `json:"factor"`
	LongSeconds   int64   `json:"long_seconds"`
	ShortSeconds  int64   `json:"short_seconds"`
	Page          bool    `json:"page"`
	LongBurn      float64 `json:"long_burn"`
	ShortBurn     float64 `json:"short_burn"`
	EffectiveBurn float64 `json:"effective_burn"`
	Firing        bool    `json:"firing"`
}

type sloPreviewResource struct {
	Config          alerting.SLOConfig  `json:"config"`
	HasTraffic      bool                `json:"has_traffic"`
	BudgetRemaining float64             `json:"budget_remaining"`
	BudgetRequests  float64             `json:"budget_window_requests"`
	Windows         []sloWindowResource `json:"windows"`
	Tiers           []sloTierResource   `json:"tiers"`
	Firing          bool                `json:"firing"`
	Page            bool                `json:"page"`
	MaxBurn         float64             `json:"max_burn"`
}

func toSLOPreview(st alerting.SLOStatus) sloPreviewResource {
	out := sloPreviewResource{
		Config: st.Config, HasTraffic: st.HasTraffic, BudgetRemaining: st.BudgetRemaining,
		BudgetRequests: st.BudgetRequests, Firing: st.Firing, Page: st.Page, MaxBurn: st.MaxBurn,
		Windows: make([]sloWindowResource, 0, len(st.Windows)), Tiers: make([]sloTierResource, 0, len(st.Tiers)),
	}
	for _, w := range st.Windows {
		out.Windows = append(out.Windows, sloWindowResource{WindowSeconds: int64(w.Window.Seconds()), Requests: w.Requests, Bad: w.Bad, BurnRate: w.Burn})
	}
	for _, t := range st.Tiers {
		out.Tiers = append(out.Tiers, sloTierResource{
			Name: t.Name, Factor: t.Factor, LongSeconds: int64(t.Long.Seconds()), ShortSeconds: int64(t.Short.Seconds()),
			Page: t.Page, LongBurn: t.LongBurn, ShortBurn: t.ShortBurn, EffectiveBurn: t.Effective, Firing: t.Firing,
		})
	}
	return out
}
