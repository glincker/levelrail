package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// defaultRequestSummaryWindow is the lookback of the request summary shown on
// GET /api/v1/apps/{name}.
const defaultRequestSummaryWindow = 5 * time.Minute

// targetRequestPoints is how many points an unset step aims for.
const targetRequestPoints = 120

type requestsResponse struct {
	App         string                   `json:"app"`
	From        time.Time                `json:"from"`
	To          time.Time                `json:"to"`
	StepSeconds float64                  `json:"step_seconds"`
	Summary     telemetry.RequestSummary `json:"summary"`
	Points      []telemetry.RequestPoint `json:"points"`
	// PreviousPoints and PreviousSummary cover the equal-length window before
	// From, shifted forward so they overlay Points, when compare=previous.
	PreviousPoints  []telemetry.RequestPoint  `json:"previous_points,omitempty"`
	PreviousSummary *telemetry.RequestSummary `json:"previous_summary,omitempty"`
}

// WithRequestSummaryWindow sets the lookback of the request summary attached
// to GET /api/v1/apps/{name}; non-positive keeps the 5 minute default.
func WithRequestSummaryWindow(d time.Duration) Option {
	return func(rt *Router) { rt.requestSummaryWindow = d }
}

func (rt *Router) summaryWindow() time.Duration {
	if rt.requestSummaryWindow > 0 {
		return rt.requestSummaryWindow
	}
	return defaultRequestSummaryWindow
}

// requestSummaryFor returns the app's recent request summary, or nil when
// telemetry is not configured or the query fails.
func (rt *Router) requestSummaryFor(r *http.Request, name string) *telemetry.RequestSummary {
	if rt.telemetry == nil {
		return nil
	}
	sum, err := telemetry.SummarizeRequests(r.Context(), rt.telemetry, name, rt.summaryWindow(), time.Now())
	if err != nil {
		rt.logger.Warn("api: request summary unavailable", slog.String("error", err.Error()), slog.String("name", name))
		if !sum.HasTraffic {
			return nil
		}
	}
	return &sum
}

// handleQueryRequests handles GET /api/v1/apps/{name}/requests: the app's
// request rate, 4xx/5xx error rates and latency percentiles derived from the
// ingress, bucketed by step (from, to and step follow the metrics endpoint).
func (rt *Router) handleQueryRequests(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, found, err := rt.lookupAppResource(r.Context(), name); err != nil {
		rt.logger.Error("api: query requests: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	step, err := parseStep(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxPoints, err := parseMaxPoints(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if step <= 0 {
		step = defaultRequestStep(to.Sub(from))
		if maxPoints > 0 {
			step = telemetry.AutoStep(to.Sub(from), maxPoints, telemetry.MinRequestStep)
		}
	} else if widened, _ := capStep(step, to.Sub(from), maxPoints); widened > 0 {
		step = widened
	}

	points, err := telemetry.QueryRequests(r.Context(), rt.telemetry, name, from, to, step)
	if err != nil {
		if points == nil {
			rt.logger.Error("api: query requests failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		rt.logger.Warn("api: query requests: partial result", slog.String("error", err.Error()), slog.String("name", name))
	}
	if points == nil {
		points = []telemetry.RequestPoint{}
	}

	summary, err := telemetry.SummarizeRequests(r.Context(), rt.telemetry, name, to.Sub(from), to)
	if err != nil {
		rt.logger.Warn("api: query requests: summary unavailable", slog.String("error", err.Error()), slog.String("name", name))
	}
	resp := requestsResponse{
		App: name, From: from, To: to, StepSeconds: step.Seconds(), Summary: summary, Points: points,
	}
	if r.URL.Query().Get("compare") == "previous" {
		span := to.Sub(from)
		prev, perr := telemetry.QueryRequests(r.Context(), rt.telemetry, name, from.Add(-span), from, step)
		if perr != nil && prev == nil {
			rt.logger.Warn("api: query requests: previous period unavailable", slog.String("error", perr.Error()), slog.String("name", name))
		}
		for i := range prev {
			prev[i].Timestamp = prev[i].Timestamp.Add(span)
		}
		resp.PreviousPoints = prev
		if ps, serr := telemetry.SummarizeRequests(r.Context(), rt.telemetry, name, span, from); serr == nil {
			resp.PreviousSummary = &ps
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func defaultRequestStep(span time.Duration) time.Duration {
	step := (span / targetRequestPoints).Truncate(telemetry.MinRequestStep)
	if step < telemetry.MinRequestStep {
		return telemetry.MinRequestStep
	}
	return step
}
