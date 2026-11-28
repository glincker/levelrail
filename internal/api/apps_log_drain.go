package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/glincker/levelrail/internal/fielddesc"
	"github.com/glincker/levelrail/internal/store"
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
	// HeaderName is returned; the value never is, only whether one is stored.
	HeaderName string `json:"header_name,omitempty"`
	HeaderSet  bool   `json:"header_set,omitempty"`
}

// setLogDrainRequest is handleSetAppLogDrain's request body.
type setLogDrainRequest struct {
	Type    store.LogDrainType `json:"type"`
	Target  string             `json:"target"`
	Enabled bool               `json:"enabled"`
	// HeaderName empty clears the header. HeaderValue is write only: empty keeps the stored value.
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
}

func toLogDrainResource(name string, d store.LogDrain) logDrainResource {
	return logDrainResource{AppName: name, Type: d.Type, Target: d.Target, Enabled: d.Enabled, HeaderName: d.HeaderName, HeaderSet: d.HeaderName != ""}
}

// validLogDrainHeaderName accepts an HTTP token and refuses names that would break framing.
func validLogDrainHeaderName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, c := range name {
		ok := c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !ok {
			return false
		}
	}
	switch strings.ToLower(name) {
	case "host", "content-length", "content-type", "transfer-encoding", "connection":
		return false
	}
	return true
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
		rt.internalError(w, "api: get app log drain failed", err, slog.String("name", name))
		return
	}
	if svc.LogDrain == nil {
		writeError(w, http.StatusNotFound, "no log drain configured for this app")
		return
	}

	res := toLogDrainResource(name, *svc.LogDrain)
	res.Target = rt.notifyTargetFor(r, "", res.Target)
	writeJSON(w, http.StatusOK, res)
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
	if res, ok := fielddesc.Get(fielddesc.LogDrain); ok {
		if msg := fielddesc.FirstProblem(res.Validate(map[string]string{"type": string(req.Type), "target": req.Target})); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}

	// Check the app first so no secret is written for a name that does not exist.
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app log drain failed", err, slog.String("name", name))
		return
	}

	drain := store.LogDrain{Type: req.Type, Target: req.Target, Enabled: req.Enabled}
	if !rt.applyLogDrainHeader(w, r, name, &drain, req, svc.LogDrain) {
		return
	}
	if err := rt.apps.UpdateServiceLogDrain(r.Context(), name, &drain); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app log drain failed", err, slog.String("name", name))
		return
	}

	writeJSON(w, http.StatusOK, toLogDrainResource(name, drain))
}

// applyLogDrainHeader stores or clears the optional auth header and reports whether the request may go on.
// The value goes only to the secrets store; a blank value keeps what is stored.
func (rt *Router) applyLogDrainHeader(w http.ResponseWriter, r *http.Request, name string, drain *store.LogDrain, req setLogDrainRequest, prev *store.LogDrain) bool {
	ns := store.LogDrainSecretService(name)
	if req.HeaderName == "" || req.Type != store.LogDrainHTTP {
		if req.HeaderValue != "" {
			writeError(w, http.StatusBadRequest, "header_value needs header_name on an http drain")
			return false
		}
		if rt.secrets != nil {
			if err := rt.secrets.DeleteAll(r.Context(), ns); err != nil {
				rt.internalError(w, "api: clear log drain header failed", err, slog.String("name", name))
				return false
			}
		}
		return true
	}
	if !validLogDrainHeaderName(req.HeaderName) {
		writeError(w, http.StatusBadRequest, "header_name must be a plain header name such as Authorization")
		return false
	}
	if strings.ContainsAny(req.HeaderValue, "\r\n") {
		writeError(w, http.StatusBadRequest, "header_value must be a single line")
		return false
	}
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured, set the master key to store a drain header")
		return false
	}
	if req.HeaderValue == "" {
		have, err := rt.secrets.Exists(r.Context(), ns, store.LogDrainSecretKey)
		if err != nil {
			rt.internalError(w, "api: check log drain header failed", err, slog.String("name", name))
			return false
		}
		if !have {
			writeError(w, http.StatusBadRequest, "header_value is required with a new header_name")
			return false
		}
		// A stored value must never follow a changed destination.
		if prev == nil || prev.Type != req.Type || prev.Target != req.Target {
			writeError(w, http.StatusBadRequest, "header_value is required when the target changes")
			return false
		}
	} else if err := rt.secrets.SetValueGuarded(r.Context(), ns, store.LogDrainSecretKey, req.HeaderValue, true); err != nil {
		rt.internalError(w, "api: store log drain header failed", err, slog.String("name", name))
		return false
	}
	drain.HeaderName = req.HeaderName
	return true
}

// handleClearAppLogDrain handles DELETE /api/v1/apps/{name}/log-drain:
// stops forwarding this app's logs externally, leaving the node-local
// store and live tail untouched.
func (rt *Router) handleClearAppLogDrain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(r.Context(), store.LogDrainSecretService(name)); err != nil {
			rt.internalError(w, "api: clear log drain header failed", err, slog.String("name", name))
			return
		}
	}
	if err := rt.apps.UpdateServiceLogDrain(r.Context(), name, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: clear app log drain failed", err, slog.String("name", name))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetLogDrainFields handles GET /api/v1/log-drains/field-descriptors: the field registry the CLI,
// the dashboard card and the docs are built from.
func (rt *Router) handleGetLogDrainFields(w http.ResponseWriter, _ *http.Request) {
	res, ok := fielddesc.Get(fielddesc.LogDrain)
	if !ok {
		rt.internalError(w, "field descriptors", errors.New("field descriptors unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
