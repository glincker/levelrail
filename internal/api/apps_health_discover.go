package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/kit/probe"
)

// Probes every candidate path for real against the running container; never a guessed result.

// Every candidate is tried and reported, so a guess is never shown as confirmed.
var discoverHealthCandidatePaths = []string{
	"/healthz", "/health", "/api/health", "/ping", "/status", "/ready", "/readyz", "/",
}

const (
	// discoverHealthPerPathTimeout bounds one candidate path's attempt.
	discoverHealthPerPathTimeout = 2 * time.Second
	// discoverHealthTotalBudget bounds the whole request: len(discoverHealthCandidatePaths)
	// attempts at discoverHealthPerPathTimeout each, plus headroom, never
	// an indefinite wait on a wedged container.
	discoverHealthTotalBudget = 20 * time.Second
)

// healthDiscoveryAttempt is one candidate path's real, tried outcome.
type healthDiscoveryAttempt struct {
	Path string `json:"path"`
	// Success means a 2xx answer; Error names the status code on a mismatch.
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

// Found is set only when exactly one candidate succeeded; callers decide how to show other counts.
type healthDiscoveryResponse struct {
	Name     string                   `json:"name"`
	Attempts []healthDiscoveryAttempt `json:"attempts"`
	Found    string                   `json:"found,omitempty"`
}

// Returns 409 with no port or no running container, and 501 when no NodeRuntimeResolver is configured.
func (rt *Router) handleDiscoverAppHealth(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, ok := rt.loadExecApp(w, r, name)
	if !ok {
		return
	}
	if svc.Port == 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("app %q has no port configured, nothing to probe", name))
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "health discovery is not configured on this control plane")
		return
	}

	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Error("api: discover app health: resolve node runtime failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", svc.NodeID))
		writeError(w, http.StatusBadGateway, "app's node is not currently reachable")
		return
	}

	target := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
	inspectCtx, cancel := context.WithTimeout(r.Context(), dockerInspectTimeout)
	state, err := nodeRuntime.InspectByName(inspectCtx, target)
	cancel()
	if err != nil {
		rt.logger.Error("api: discover app health: inspect container failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("container", target))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if state == nil || !state.Running {
		writeError(w, http.StatusConflict, "app has no running container")
		return
	}

	addr, err := application.PrimaryAddr(state, svc.Port)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, discoverAppHealth(r.Context(), name, addr))
}

// discoverAppHealth runs the actual probing, split out from the HTTP
// handler so it is testable without an httptest round trip per case.
func discoverAppHealth(ctx context.Context, name, addr string) healthDiscoveryResponse {
	budgetCtx, cancel := context.WithTimeout(ctx, discoverHealthTotalBudget)
	defer cancel()

	prober := probe.New(nil, nil, probe.Limits{DefaultTimeout: discoverHealthPerPathTimeout})
	target := probe.Target{Addr: addr}

	attempts := make([]healthDiscoveryAttempt, 0, len(discoverHealthCandidatePaths))
	var successes []string
	for _, path := range discoverHealthCandidatePaths {
		start := time.Now()
		attemptErr := prober.Check(budgetCtx, target, probe.Config{Path: path, Timeout: discoverHealthPerPathTimeout})
		a := healthDiscoveryAttempt{Path: path, LatencyMs: time.Since(start).Milliseconds()}
		if attemptErr == nil {
			a.Success = true
			successes = append(successes, path)
		} else {
			a.Error = attemptErr.Error()
		}
		attempts = append(attempts, a)
	}

	resp := healthDiscoveryResponse{Name: name, Attempts: attempts}
	if len(successes) == 1 {
		resp.Found = successes[0]
	}
	return resp
}
