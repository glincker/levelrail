package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// VolumeCloneRestoreHistoryStore is the store surface the volume
// clone-restore history handler needs, the app service volume counterpart
// of CloneRestoreHistoryStore (database_clone_restore.go).
type VolumeCloneRestoreHistoryStore interface {
	ListVolumeCloneRestores(ctx context.Context, serviceName, volumeName string) ([]store.VolumeCloneRestore, error)
}

// VolumeCloneRestoreRunner is the surface the volume clone-restore trigger
// handler needs from internal/backup.VolumeCloneRestoreRunner, the app
// service volume counterpart of CloneRestoreRunner. *backup.
// VolumeCloneRestoreRunner satisfies this structurally, the same boundary
// CloneRestoreRunner's own doc comment describes.
type VolumeCloneRestoreRunner interface {
	RunVolumeCloneRestore(ctx context.Context, historyID, sourceServiceName, sourceVolumeName, newVolumeName, backupHistoryID string) error
}

// volumeCloneRestoreResource is the wire shape for one "restore as new
// volume" attempt.
type volumeCloneRestoreResource struct {
	ID                string `json:"id"`
	SourceServiceName string `json:"source_service_name"`
	SourceVolumeName  string `json:"source_volume_name"`
	NewVolumeName     string `json:"new_volume_name"`
	BackupHistoryID   string `json:"backup_history_id"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at,omitempty"`
}

func toVolumeCloneRestoreResource(h store.VolumeCloneRestore) volumeCloneRestoreResource {
	return volumeCloneRestoreResource{
		ID:                h.ID,
		SourceServiceName: h.SourceServiceName,
		SourceVolumeName:  h.SourceVolumeName,
		NewVolumeName:     h.NewVolumeName,
		BackupHistoryID:   h.BackupHistoryID,
		Status:            h.Status,
		Error:             h.Error,
		StartedAt:         h.StartedAt,
		FinishedAt:        h.FinishedAt,
	}
}

// volumeCloneRestoreRequest is
// POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new's body.
// NewVolumeName is optional: left blank, defaultVolumeCloneName mints one.
type volumeCloneRestoreRequest struct {
	BackupID      string `json:"backup_id"`
	NewVolumeName string `json:"new_volume_name,omitempty"`
}

// handleVolumeCloneRestore handles
// POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new: creates a
// brand-new, standalone Docker volume and restores a previously succeeded
// backup of {name}'s {volume} into it, never touching that volume's own
// live contents. The app service volume counterpart of handleCloneRestore
// (database_clone_restore.go), with one real difference: an app service
// volume has no "create a new resource" path the way a database does
// (store.ServiceVolume only exists as an entry in a service's own app.yaml
// declared volumes, see ServiceVolumeDockerName's own doc comment), so the
// new volume this creates is a bare Docker volume, not attached to any
// service or app.yaml, and not visible anywhere else in this API except
// through this route's own history.
//
// AbilityWriteSensitive, not AbilityRoot, the identical reasoning
// handleCloneRestore's own doc comment gives: this never overwrites
// anything, it only creates a new volume and consumes a previously stored
// backup-target credential to populate it.
func (rt *Router) handleVolumeCloneRestore(w http.ResponseWriter, r *http.Request) {
	if rt.volumeCloneRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: volume clone restore: load service failed"); !ok {
		return
	}

	var req volumeCloneRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BackupID == "" {
		writeError(w, http.StatusBadRequest, "backup_id is required")
		return
	}

	backup, err := rt.backupHistory.GetBackupHistory(r.Context(), req.BackupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: volume clone restore: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", req.BackupID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if backup.ServiceName != serviceName || backup.VolumeName != volumeName {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was not taken from %s/%s", req.BackupID, serviceName, volumeName))
		return
	}
	if backup.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be restored from", req.BackupID, backup.Status))
		return
	}

	newVolumeName := strings.TrimSpace(req.NewVolumeName)
	if newVolumeName == "" {
		newVolumeName, err = defaultVolumeCloneName(serviceName, volumeName)
		if err != nil {
			rt.logger.Error("api: volume clone restore: generate default volume name failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	historyID, err := randomVolumeCloneRestoreID()
	if err != nil {
		rt.logger.Error("api: volume clone restore: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleCloneRestore's own goroutine gives
		if err := rt.volumeCloneRestoreRunner.RunVolumeCloneRestore(context.Background(), historyID, serviceName, volumeName, newVolumeName, req.BackupID); err != nil {
			rt.logger.Error("api: volume clone restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("service", serviceName), slog.String("volume", volumeName))
		}
	}()

	writeJSON(w, http.StatusAccepted, volumeCloneRestoreResource{
		ID:                historyID,
		SourceServiceName: serviceName,
		SourceVolumeName:  volumeName,
		NewVolumeName:     newVolumeName,
		BackupHistoryID:   req.BackupID,
		Status:            store.BackupStatusRunning,
	})
}

// handleListVolumeCloneRestores handles
// GET /api/v1/apps/{name}/volumes/{volume}/clone-restores: past "restore
// as new volume" attempts sourced from {name}'s {volume}, newest first,
// the app service volume counterpart of handleListCloneRestores.
func (rt *Router) handleListVolumeCloneRestores(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: list volume clone restores: load service failed"); !ok {
		return
	}

	history, err := rt.volumeCloneRestoreHistory.ListVolumeCloneRestores(r.Context(), serviceName, volumeName)
	if err != nil {
		rt.logger.Error("api: list volume clone restores failed", slog.String("error", err.Error()), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]volumeCloneRestoreResource, 0, len(history))
	for _, h := range history {
		out = append(out, toVolumeCloneRestoreResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// defaultVolumeCloneName mints a new, standalone Docker volume name when
// an operator doesn't supply one: "clone-" plus the source service and
// volume so the new volume's own lineage is legible from `docker volume
// ls` alone, plus a random suffix so two clone-restores of the same
// source never collide.
func defaultVolumeCloneName(serviceName, volumeName string) (string, error) {
	suffix, err := randomHexSuffix()
	if err != nil {
		return "", fmt.Errorf("generate default volume name: %w", err)
	}
	return "clone-" + serviceName + "-" + volumeName + "-" + suffix, nil
}

func randomHexSuffix() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// randomVolumeCloneRestoreID mirrors randomCloneRestoreID's exact shape,
// with its own "vcr_" prefix so a volume clone-restore ID is never
// visually confusable with a database clone-restore ID ("clr_") or a
// restore-history ID ("rsh_") in a log line or error message.
func randomVolumeCloneRestoreID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate volume clone restore id: %w", err)
	}
	return "vcr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
