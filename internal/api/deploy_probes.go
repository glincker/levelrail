package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// probeAttemptResource is the per-attempt JSON shape GET
// .../deploys/{deployId}/probes returns. deploy_attempt_id is left off
// the wire, the same way deployAttemptResource drops ServiceName:
// both are already implied by the request path.
type probeAttemptResource struct {
	ID         int64  `json:"id"`
	Target     string `json:"target"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Error      string `json:"error,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	ProbedAt   string `json:"probed_at"`
}

func toProbeAttemptResource(a store.ProbeAttempt) probeAttemptResource {
	return probeAttemptResource{
		ID:         a.ID,
		Target:     a.Target,
		Success:    a.Success,
		StatusCode: a.StatusCode,
		ExitCode:   a.ExitCode,
		Error:      a.Error,
		LatencyMS:  a.LatencyMS,
		ProbedAt:   a.ProbedAt.UTC().Format(time.RFC3339Nano),
	}
}

// handleListProbeAttempts handles GET
// /api/v1/apps/{name}/deploys/{deployId}/probes: the individual
// readiness-probe attempts (internal/probe.WithOnAttempt,
// migrations/0280_probe_attempts.sql) this deploy's cutover made, the
// per-attempt detail (status code, latency) GET .../deploys/{deployId}'s
// own reconcile-condition summary never captures. Plain polling JSON,
// not SSE: a bounded, already-terminal-by-replay list the same way GET
// .../deploy-attempts itself already is, not a live tail.
func (rt *Router) handleListProbeAttempts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	deployID := r.PathValue("deployId")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: list probe attempts: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: list probe attempts: load attempt failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if attempt.ServiceName != name {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}

	attempts, err := rt.probeAttempts.ListProbeAttempts(r.Context(), deployID)
	if err != nil {
		rt.logger.Error("api: list probe attempts failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]probeAttemptResource, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, toProbeAttemptResource(a))
	}
	writeJSON(w, http.StatusOK, out)
}
