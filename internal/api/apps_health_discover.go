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

// This file implements POST /api/v1/apps/{name}/health/discover: every
// candidate path is actually requested against the app's running
// container, never presented as working without a real response behind
// it. Separate from the reconciler's deploy-gating probe machinery: a
// single, synchronous, unrecorded diagnostic read per request.

// discoverHealthCandidatePaths is the ordered set of well-known paths
// handleDiscoverAppHealth tries. Order only affects attempts' order in
// the response; every path is tried and reported, not just the first
// match, so a guess is never presented as confirmed without a real
// response behind it.
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
	// Success is true only when the container answered with a status in
	// probe's own default expected range (2xx). Error (set only when
	// Success is false) is the probe's own failure reason, which already
	// names the actual status code for a status mismatch.
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

// healthDiscoveryResponse is handleDiscoverAppHealth's 200 body. Found is
// set only when exactly one candidate path succeeded: a caller (the
// dashboard or the CLI) decides for itself how to present zero or
// multiple successes, never auto-filling an unverified guess.
type healthDiscoveryResponse struct {
	Name     string                   `json:"name"`
	Attempts []healthDiscoveryAttempt `json:"attempts"`
	Found    string                   `json:"found,omitempty"`
}

// handleDiscoverAppHealth handles POST /api/v1/apps/{name}/health/discover.
// 409 if the app has no port (nothing to probe over HTTP) or no running
// container; 501 if no NodeRuntimeResolver is configured, the same
// "not configured" shape every other optional-dependency route in this
// package already uses (see handleExecApp).
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

	addr, err := application.PrimaryAddr(state)
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
