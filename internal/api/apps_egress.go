package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/GLINCKER/levelrail/internal/store"
)

// egressHostRe restricts allow hosts to a hostname or IPv4 literal: the
// value is later word-split by the egress sidecar's shell script.
var egressHostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// egressAllowResource is one host+port pair in an
// egressPolicyResource's allow list, the wire shape of
// store.ServiceEgressAllow.
type egressAllowResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// egressPolicyResource is the GET and PUT egress-policy body; mode and allow are omitted when no policy is set.
type egressPolicyResource struct {
	AppName string                `json:"app_name"`
	Mode    string                `json:"mode,omitempty"`
	Allow   []egressAllowResource `json:"allow,omitempty"`
}

// setEgressPolicyRequest is handleSetAppEgressPolicy's request body.
type setEgressPolicyRequest struct {
	Mode  string                `json:"mode"`
	Allow []egressAllowResource `json:"allow"`
}

func egressPolicyResourceFrom(name string, policy *store.ServiceEgressPolicy) egressPolicyResource {
	res := egressPolicyResource{AppName: name}
	if policy == nil {
		return res
	}
	res.Mode = policy.Mode
	res.Allow = make([]egressAllowResource, len(policy.Allow))
	for i, a := range policy.Allow {
		res.Allow[i] = egressAllowResource{Host: a.Host, Port: a.Port}
	}
	return res
}

// handleGetAppEgressPolicy returns the egress allowlist. Omitted mode and allow mean unrestricted egress.
func (rt *Router) handleGetAppEgressPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get app egress policy: load app failed", err, slog.String("name", name))
		return
	}

	writeJSON(w, http.StatusOK, egressPolicyResourceFrom(name, svc.Egress))
}

// handleSetAppEgressPolicy sets an app's egress allowlist. A general PUT would silently drop it, so this is a dedicated endpoint.
// Validation matches app.yaml's egress block, so it never stores a policy app.yaml would reject.
func (rt *Router) handleSetAppEgressPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setEgressPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Mode != store.EgressModeAllowlist {
		writeError(w, http.StatusBadRequest, "mode must be \""+store.EgressModeAllowlist+"\"")
		return
	}
	if len(req.Allow) == 0 {
		writeError(w, http.StatusBadRequest, "allow must have at least one entry")
		return
	}
	allow := make([]store.ServiceEgressAllow, len(req.Allow))
	for i, a := range req.Allow {
		if !egressHostRe.MatchString(a.Host) {
			writeError(w, http.StatusBadRequest, "allow entries require a hostname or IPv4 address")
			return
		}
		if a.Port < 1 || a.Port > 65535 {
			writeError(w, http.StatusBadRequest, "allow entries require a port between 1 and 65535")
			return
		}
		allow[i] = store.ServiceEgressAllow{Host: a.Host, Port: a.Port}
	}
	policy := &store.ServiceEgressPolicy{Mode: req.Mode, Allow: allow}

	if err := rt.apps.UpdateServiceEgressPolicy(r.Context(), name, policy); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app egress policy failed", err, slog.String("name", name))
		return
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, egressPolicyResourceFrom(name, policy))
}

// handleClearAppEgressPolicy opts the app back to unrestricted egress. The sidecar is torn down on the next reconcile; the app container is not touched.
func (rt *Router) handleClearAppEgressPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceEgressPolicy(r.Context(), name, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: clear app egress policy failed", err, slog.String("name", name))
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
