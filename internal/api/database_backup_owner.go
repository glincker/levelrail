package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// refuseForeignBackup writes a 409 and returns true when a backup row was taken
// by an earlier database that used the same name. Rows from before instance ids
// existed are trusted only if the database is not newer than the backup.
func refuseForeignBackup(w http.ResponseWriter, db *store.DesiredDatabase, backupID, instanceID, startedAt string) bool {
	if db.OwnsBackup(instanceID, startedAt) {
		return false
	}
	writeError(w, http.StatusConflict, fmt.Sprintf("backup %q belongs to an earlier database that used the name %q and cannot be used with this one", backupID, db.Name))
	return true
}

// refuseForeignBaseBackup is refuseForeignBackup for a base backup named by id.
// An id that is not in the database's own history is left for the runner to reject.
func (rt *Router) refuseForeignBaseBackup(w http.ResponseWriter, r *http.Request, db *store.DesiredDatabase, baseID string) bool {
	if rt.baseBackupHistory == nil {
		return false
	}
	rows, err := rt.baseBackupHistory.ListBaseBackupHistory(r.Context(), db.Name)
	if err != nil {
		rt.internalError(w, "api: check base backup owner failed", err, slog.String("name", db.Name))
		return true
	}
	for _, b := range rows {
		if b.ID == baseID {
			return refuseForeignBackup(w, db, b.ID, b.InstanceID, b.StartedAt)
		}
	}
	return false
}
