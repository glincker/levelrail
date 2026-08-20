package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// networkResource is GET /api/v1/apps/{name}/network's wire shape: the
// live traffic path from Caddy ingress down to the container, for the
// frontend's Network tab. HostPort is only ever meaningful while Running
// is true, matching docker.ContainerState.Ports' own "empty until the
// container is actually running" contract.
type networkResource struct {
	ContainerPort int  `json:"container_port"`
	HostPort      int  `json:"host_port,omitempty"`
	Running       bool `json:"running"`
}

// handleGetAppNetwork handles GET /api/v1/apps/{name}/network. Host port
// is resolved live via execRuntime, never persisted: it changes on every
// container recreate. Degrades to HostPort 0 / Running false rather than
// an error when execRuntime is nil or the lookup fails, same as
// handleListImages.
func (rt *Router) handleGetAppNetwork(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app network: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := networkResource{ContainerPort: svc.Port}

	if rt.execRuntime == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	rt2, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Warn("api: get app network: resolve node runtime failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", svc.NodeID))
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Same target-container derivation handleExecApp and
	// internal/reconcile/ingress already use to find a service's
	// currently active container from its own desired state.
	target := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)
	state, err := rt2.InspectByName(r.Context(), target)
	if err != nil || state == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Running = state.Running
	if state.Running && len(state.Ports) > 0 {
		resp.HostPort = state.Ports[0].HostPort
	}
	writeJSON(w, http.StatusOK, resp)
}
