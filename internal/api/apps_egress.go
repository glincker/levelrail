package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// egressAllowResource is one host+port pair in an
// egressPolicyResource's allow list, the wire shape of
// store.ServiceEgressAllow.
type egressAllowResource struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// egressPolicyResource is GET/PUT /api/v1/apps/{name}/egress-policy's
// response body. Mode/Allow are omitted (not zero-valued) when the app
// has no egress policy configured, the same "absent, not a zero value,
// means unconfigured" shape appStorageResource's own omitempty gives
// storage_target_id.
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

// handleGetAppEgressPolicy handles GET /api/v1/apps/{name}/egress-policy:
// reads the app's current outbound network allowlist, unconfigured
// (Mode/Allow both omitted) meaning unrestricted egress, today's
// behavior for every app that has never opted in.
func (rt *Router) handleGetAppEgressPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app egress policy: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, egressPolicyResourceFrom(name, svc.Egress))
}

// handleSetAppEgressPolicy handles PUT /api/v1/apps/{name}/egress-policy:
// sets (or replaces) this app's outbound network allowlist, the UI/CLI-
// facing equivalent of app.yaml's own egress: block for an app that
// already exists, mirroring handleSetAppVaultEnv's own "dedicated
// endpoint, not the general PUT" shape: an ordinary PUT
// /api/v1/apps/{name} would silently drop every declarative field
// (Egress included) it doesn't itself carry forward.
//
// mode must be "allowlist" (internal/spec.EgressModeAllowlist's only
// meaningful value today) with at least one allow entry, the same
// validation internal/spec.Service.validateEgress applies to app.yaml's
// own egress: block, so this endpoint can never persist a policy
// app.yaml itself would reject.
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
		if a.Host == "" {
			writeError(w, http.StatusBadRequest, "allow entries require a non-empty host")
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
		rt.logger.Error("api: set app egress policy failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, egressPolicyResourceFrom(name, policy))
}

// handleClearAppEgressPolicy handles DELETE
// /api/v1/apps/{name}/egress-policy: the reverse of
// handleSetAppEgressPolicy, opting this app back out to unrestricted
// egress. The next reconcile pass tears down the egress sidecar
// (internal/reconcile/application's reconcileEgress); an already-running
// app container itself is never touched, since egress enforcement lives
// entirely in the sidecar, not the app's own container.
func (rt *Router) handleClearAppEgressPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceEgressPolicy(r.Context(), name, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear app egress policy failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
