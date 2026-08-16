package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// cloneAppRequest is POST /api/v1/apps/{name}/clone's body.
type cloneAppRequest struct {
	NewName string `json:"new_name"`
}

// handleCloneApp handles POST /api/v1/apps/{name}/clone: duplicates
// {name}'s desired state under req.NewName, filling the Coolify-parity
// gap of re-entering an app's whole config by hand to stand up a near-
// duplicate. Conflict-detection on the new name matches handleCreateApp
// exactly (a 409, not a silent overwrite), and the write itself goes
// through the same rt.apps.SaveDesiredService path, so it inherits that
// path's own domain-claim enforcement for free.
//
// Three fields deliberately do not carry over, each for its own reason:
//
//   - Domains: the source app still owns them, so copying them verbatim
//     would always fail SaveDesiredService's own domain-claim check
//     (*store.ErrDomainTaken), and even if it somehow didn't, two
//     services answering the same Host header is exactly the silent
//     ingress-shadowing bug store.ErrDomainTaken exists to prevent (see
//     that type's own doc comment). The clone starts domainless, the
//     same state any other freshly created app starts in.
//   - SecretEnv values: the *names* do carry over (below), but never the
//     decrypted plaintext. internal/secrets scopes a DEK per service
//     name by design (see secrets.Manager.dekFor's own doc comment), so
//     duplicating decrypted values across that boundary would silently
//     double a credential's blast radius (e.g. a database password or
//     webhook secret now live under two service identities) without the
//     operator ever making that call explicitly. The reconciler already
//     has a safe, existing failure mode for a declared-but-unset secret
//     (resolveEnv's own doc comment: a required one fails the deploy
//     loudly with a condition, an optional one is just omitted), so
//     leaving the clone's values unset is not a new gap, it reuses one
//     already-correct path. The operator re-sets them via the existing
//     PUT /api/v1/apps/{name}/secrets/{key}.
//   - NodeID: SaveDesiredService never writes it on any call, including
//     this one (see that method's own doc comment), so the clone starts
//     on the local node exactly like every other brand-new app. This
//     also avoids silently doubling resource load on whatever node the
//     source happened to be pinned to; placement is always a deliberate
//     follow-up via PUT /api/v1/apps/{name}/node, never bundled into a
//     creation-shaped call, the same boundary handleCreateApp's own doc
//     comment establishes for NodeID.
//
// ProjectID is the opposite: it does carry over automatically, the same
// "safe at create time" exception handleCreateApp's own doc comment
// documents for req.ProjectID. Project assignment is a pure
// organizational label with no capacity or security implication (unlike
// node placement), so a clone landing in the same project as its source
// is the low-surprise default; source.ProjectID is already a validated,
// live value by construction (store.DB.DeleteProject sets it back to
// NULL on delete, migrations/0022's own comment), so no re-validation
// against the project registry is needed the way validateProjectID does
// for client-supplied input.
//
// Like handleCreateApp, this never triggers a deploy or otherwise
// touches the reconciler directly: SaveDesiredService alone is enough,
// the reconciler's own next sync (cmd/levelrail/main.go) discovers the
// new desired_services row and spins up its controller the same way it
// already does for every other row, see application.New's own doc
// comment.
func (rt *Router) handleCloneApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req cloneAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.NewName == "" {
		writeError(w, http.StatusBadRequest, "new_name is required")
		return
	}

	source, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: clone app: load source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Same existence check handleCreateApp runs, and it also covers
	// req.NewName == name for free: the source itself is found under
	// that name, so this correctly rejects a self-clone as a conflict
	// rather than silently overwriting the source in place.
	_, err = rt.apps.GetDesiredService(r.Context(), req.NewName)
	if err == nil {
		writeError(w, http.StatusConflict, "an app with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: clone app: check new name failed", slog.String("error", err.Error()), slog.String("new_name", req.NewName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	clone := store.DesiredService{
		Name:      req.NewName,
		Image:     source.Image,
		Port:      source.Port,
		Env:       source.Env,
		SecretEnv: source.SecretEnv,
		Resources: source.Resources,
		Health:    source.Health,
		Strategy:  source.Strategy,
		Replicas:  source.Replicas,
	}
	if err := rt.apps.SaveDesiredService(r.Context(), clone); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.logger.Error("api: clone app failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("new_name", req.NewName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if source.ProjectID != "" {
		if err := rt.apps.UpdateServiceProject(r.Context(), req.NewName, source.ProjectID); err != nil {
			rt.logger.Error("api: clone app: assign project failed", slog.String("error", err.Error()), slog.String("new_name", req.NewName), slog.String("project_id", source.ProjectID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	cloned, err := rt.apps.GetDesiredService(r.Context(), req.NewName)
	if err != nil {
		rt.logger.Error("api: clone app: reload after clone failed", slog.String("error", err.Error()), slog.String("new_name", req.NewName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toAppResource(*cloned))
}
