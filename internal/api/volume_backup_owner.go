package api

import (
	"context"
	"net/http"

	"github.com/glincker/levelrail/internal/store"
)

// volumeBackupOwner is optional on the history store so test fakes need not implement it.
type volumeBackupOwner interface {
	OwnsVolumeBackup(ctx context.Context, serviceName, rowInstanceID string) (bool, error)
}

// ownsVolumeBackup is false for a row taken by an earlier app that used the same name.
// Only root may use a row no app instance owns, so a newcomer never inherits it.
func (rt *Router) ownsVolumeBackup(r *http.Request, serviceName string, h store.BackupHistory) bool {
	o, ok := rt.backupHistory.(volumeBackupOwner)
	if !ok {
		return true
	}
	owned, err := o.OwnsVolumeBackup(r.Context(), serviceName, h.InstanceID)
	if err != nil {
		return false
	}
	return owned || (h.InstanceID == "" && rt.callerIsRoot(r))
}

// refuseForeignVolumeBackup answers like a missing backup so the id reveals nothing.
func (rt *Router) refuseForeignVolumeBackup(w http.ResponseWriter, r *http.Request, serviceName string, h store.BackupHistory) bool {
	if rt.ownsVolumeBackup(r, serviceName, h) {
		return false
	}
	writeError(w, http.StatusNotFound, "backup not found")
	return true
}
