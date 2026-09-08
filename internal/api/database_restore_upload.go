package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultMaxRestoreUploadBytes caps a raw restore-upload request body
// when the control plane hasn't overridden it via
// WithMaxRestoreUploadBytes. 5GiB comfortably covers a plain-SQL or
// mongodump archive for the small, self-hosted-scale databases this
// platform targets, without leaving the limit effectively unbounded.
const defaultMaxRestoreUploadBytes = 5 << 30

// handleTriggerRestoreUpload handles
// POST /api/v1/databases/{name}/restore-upload: restores name from a raw
// dump file the request body carries directly, not from a backup this
// platform already took (that's handleTriggerRestore's job). The real
// use case is a first-time migration of an existing external database
// into this platform, or applying a manually-taken dump, neither of
// which has a backup_history row to name.
//
// This does not follow handleTriggerRestore's own detached-goroutine
// pattern: that handler can return immediately because it has nothing
// left to read from the request, but this one's entire input *is* the
// request body. Detaching the restore into a goroutine while returning
// early would let net/http close the connection under the client's
// upload, cutting it off mid-stream. So this handler streams the body
// into the restore straight through and blocks until it's actually
// finished, returning the real outcome (200) or a real error, never a
// 202-and-poll placeholder.
//
// AbilityRoot, matching handleTriggerRestore's own gate exactly: this is
// the identical in-place, irreversible overwrite, just fed from an
// uploaded file instead of a stored backup.
func (rt *Router) handleTriggerRestoreUpload(w http.ResponseWriter, r *http.Request) {
	if rt.restoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: trigger restore upload: load database failed")
	if !ok {
		return
	}

	maxBytes := rt.restoreUploadMaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxRestoreUploadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	historyID, err := randomRestoreHistoryID()
	if err != nil {
		rt.logger.Error("api: trigger restore upload: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	containerName := databaseContainerName(name)
	runErr := rt.restoreRunner.RunRestoreFromReader(r.Context(), historyID, name, db.Engine, containerName, r.Body)
	if runErr != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(runErr, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("uploaded file exceeds the %d byte limit", maxBytesErr.Limit))
			return
		}
		rt.logger.Error("api: restore from uploaded file failed", slog.String("error", runErr.Error()), slog.String("id", historyID), slog.String("database", name))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("restore failed: %s", runErr.Error()))
		return
	}

	writeJSON(w, http.StatusOK, restoreHistoryResource{
		ID:           historyID,
		DatabaseName: name,
		Status:       store.BackupStatusSucceeded,
	})
}
