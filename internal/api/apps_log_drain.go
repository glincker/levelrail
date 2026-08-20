package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// logDrainResource is GET/PUT /api/v1/apps/{name}/log-drain's response
// body: the app name plus store.LogDrain's own fields, the same
// "deliberately its own small type, not a reuse of appResource" shape
// appStorageResource already establishes for the equivalent storage
// route.
type logDrainResource struct {
	AppName string             `json:"app_name"`
	Type    store.LogDrainType `json:"type"`
	Target  string             `json:"target"`
	Enabled bool               `json:"enabled"`
}

// setLogDrainRequest is handleSetAppLogDrain's request body.
type setLogDrainRequest struct {
	Type    store.LogDrainType `json:"type"`
	Target  string             `json:"target"`
	Enabled bool               `json:"enabled"`
}

func toLogDrainResource(name string, d store.LogDrain) logDrainResource {
	return logDrainResource{AppName: name, Type: d.Type, Target: d.Target, Enabled: d.Enabled}
}

// handleGetAppLogDrain handles GET /api/v1/apps/{name}/log-drain: the
// app's currently configured external log-forwarding sink, 404 if the
// app doesn't exist or has none configured (a caller can't tell those
// two 404 cases apart from status code alone today, the same shape
// handleGetApp's own 404 already has for a missing app).
func (rt *Router) handleGetAppLogDrain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: get app log drain failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if svc.LogDrain == nil {
		writeError(w, http.StatusNotFound, "no log drain configured for this app")
		return
	}

	writeJSON(w, http.StatusOK, toLogDrainResource(name, *svc.LogDrain))
}

// handleSetAppLogDrain handles PUT /api/v1/apps/{name}/log-drain:
// forwards this app's container log stream to an external HTTP endpoint
// or syslog target, in addition to (never instead of) the existing
// node-local store (internal/telemetry.DrainForwarder taps
// LogBroadcaster the same way a live SSE viewer does, see that
// package's own doc comment).
func (rt *Router) handleSetAppLogDrain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setLogDrainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type != store.LogDrainHTTP && req.Type != store.LogDrainSyslog {
		writeError(w, http.StatusBadRequest, "type must be \"http\" or \"syslog\"")
		return
	}
	if req.Type == store.LogDrainHTTP && req.Target == "" {
		writeError(w, http.StatusBadRequest, "target is required for an http drain")
		return
	}

	drain := store.LogDrain{Type: req.Type, Target: req.Target, Enabled: req.Enabled}
	if err := rt.apps.UpdateServiceLogDrain(r.Context(), name, &drain); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app log drain failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toLogDrainResource(name, drain))
}

// handleClearAppLogDrain handles DELETE /api/v1/apps/{name}/log-drain:
// stops forwarding this app's logs externally, leaving the node-local
// store and live tail untouched.
func (rt *Router) handleClearAppLogDrain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceLogDrain(r.Context(), name, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear app log drain failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
