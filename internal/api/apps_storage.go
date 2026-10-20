package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// appStorageResource is PUT/DELETE /api/v1/apps/{name}/storage's response
// body: just the two fields that endpoint actually changes, the same
// "deliberately its own small type, not a reuse of appResource" reasoning
// databasePublicAccessResource's own doc comment gives
// (database_public_access.go).
type appStorageResource struct {
	AppName         string `json:"app_name"`
	StorageTargetID string `json:"storage_target_id,omitempty"`
}

// setAppStorageRequest is handleSetAppStorage's request body.
type setAppStorageRequest struct {
	StorageTargetID string `json:"storage_target_id"`
}

// handleSetAppStorage handles PUT /api/v1/apps/{name}/storage: attaches an
// already-connected backup target (the same S3-compatible bucket
// connection an operator may already use for a database's scheduled
// backups, backup_targets.go) to this app as its object-storage
// credential source. internal/reconcile/application's controller resolves
// the target's Endpoint/Region/Bucket plus its credentials into S3_* env
// vars at container-create time once this is set, see that package's
// resolveEnv doc comment.
//
// The target must already exist: a typo'd or already-deleted
// storage_target_id fails loudly here with a 400 rather than silently
// being written and only surfacing as a reconcile failure later, the same
// "validate against the real registry first" shape handleSetAppNode/
// handleSetAppProject already establish for node_id/project_id.
func (rt *Router) handleSetAppStorage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppStorageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.StorageTargetID == "" {
		writeError(w, http.StatusBadRequest, "storage_target_id is required")
		return
	}

	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), req.StorageTargetID); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusBadRequest, "unknown storage_target_id")
		return
	} else if err != nil {
		rt.logger.Error("api: set app storage: look up backup target failed", slog.String("error", err.Error()), slog.String("storage_target_id", req.StorageTargetID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.UpdateServiceStorageTarget(r.Context(), name, req.StorageTargetID); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app storage failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, appStorageResource{AppName: name, StorageTargetID: req.StorageTargetID})
}

// handleClearAppStorage handles DELETE /api/v1/apps/{name}/storage: the
// reverse of handleSetAppStorage, detaching whatever backup target this
// app currently resolves object-storage credentials from. The next
// reconcile pass sees storage_target_id go back to empty and stops
// injecting S3_* env vars into freshly (re)created containers; an
// already-running container keeps whatever env it was created with until
// its next deploy or restart, the same "desired state changes, running
// containers converge on their own schedule" behavior every other
// reconciled field in this codebase already has.
func (rt *Router) handleClearAppStorage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceStorageTarget(r.Context(), name, ""); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear app storage failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
