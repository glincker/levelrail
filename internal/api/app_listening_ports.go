package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Listen verdicts for the domain wizard's container port step.
const (
	listenVerdictListening    = "listening"
	listenVerdictOtherPort    = "other_port"
	listenVerdictNotListening = "not_listening"
	listenVerdictUnknown      = "unknown"
)

// Reasons a listen probe could not run.
const (
	listenReasonNoRuntime    = "no_runtime"
	listenReasonExecDisabled = "exec_disabled"
	listenReasonNotRunning   = "not_running"
	listenReasonProbeFailed  = "probe_failed"
)

type listeningPortsResource struct {
	App string `json:"app"`
	// Port is the container port the ingress dials.
	Port int `json:"port"`
	// Probed is true when the container's own socket table was read.
	Probed    bool   `json:"probed"`
	Reason    string `json:"reason,omitempty"`
	Listening []int  `json:"listening"`
	Verdict   string `json:"verdict"`
}

// listenVerdict compares the configured port with the sockets found.
func listenVerdict(port int, listening []int) string {
	switch {
	case slices.Contains(listening, port):
		return listenVerdictListening
	case len(listening) > 0:
		return listenVerdictOtherPort
	}
	return listenVerdictNotListening
}

// handleAppListeningPorts handles GET /api/v1/apps/{name}/listening-ports:
// whether the running container actually listens on the configured port.
// Needs exec enabled for the app, since it reads /proc/net/tcp inside.
func (rt *Router) handleAppListeningPorts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: listening ports: load app failed", err, slog.String("name", name))
		return
	}
	out := listeningPortsResource{App: name, Port: svc.Port, Listening: []int{}, Verdict: listenVerdictUnknown}
	switch {
	case rt.execRuntime == nil:
		out.Reason = listenReasonNoRuntime
	case !svc.ExecEnabled:
		out.Reason = listenReasonExecDisabled
	default:
		rt.probeListening(r.Context(), svc, &out)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) probeListening(ctx context.Context, svc *store.DesiredService, out *listeningPortsResource) {
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Warn("api: listening ports: resolve node runtime failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		out.Reason = listenReasonProbeFailed
		return
	}
	pctx, cancel := context.WithTimeout(ctx, envDurationOr(envDiagnoseProbeBudget, defaultProbeBudget))
	defer cancel()
	container := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
	state, err := runtime.InspectByName(pctx, container)
	if err != nil {
		rt.logger.Warn("api: listening ports: inspect failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		out.Reason = listenReasonProbeFailed
		return
	}
	if state == nil || !state.Running {
		out.Reason = listenReasonNotRunning
		return
	}
	ports, ok := rt.readListeningPorts(pctx, runtime, container)
	if !ok {
		out.Reason = listenReasonProbeFailed
		return
	}
	if ports != nil {
		out.Listening = ports
	}
	out.Probed = true
	out.Verdict = listenVerdict(svc.Port, out.Listening)
}
