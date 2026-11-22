package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// restoreTestScanLimit bounds how far back the newest succeeded backup is searched for.
const restoreTestScanLimit = 50

// restoreTestResource is the latest restore test of one database, read from the drill history.
type restoreTestResource struct {
	ID         string `json:"id"`
	BackupID   string `json:"backup_id"`
	Status     string `json:"status"`
	Stage      string `json:"stage,omitempty"`
	Bytes      int64  `json:"bytes"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// handleStartDatabaseRestoreTest handles POST /api/v1/databases/{name}/restore-test:
// restores the newest succeeded backup into a throwaway container, never the live database.
func (rt *Router) handleStartDatabaseRestoreTest(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "restore tests are not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, ok := rt.loadDatabaseForRunner(w, r, name, "api: restore test: load database failed"); !ok {
		return
	}
	rows, err := rt.backupHistory.ListBackupHistory(r.Context(), name, restoreTestScanLimit, nil)
	if err != nil {
		rt.internalError(w, "api: restore test: list backups failed", err, slog.String("name", name))
		return
	}
	var latest *store.BackupHistory
	for i := range rows {
		if rows[i].Status == store.BackupStatusSucceeded {
			latest = &rows[i]
			break
		}
	}
	if latest == nil {
		writeError(w, http.StatusNotFound, "no succeeded backup to test")
		return
	}
	if t, terr := rt.backupTargets.GetBackupTarget(r.Context(), latest.TargetID); terr == nil && (strings.HasPrefix(t.Endpoint, store.LocalEndpointScheme) || strings.HasPrefix(t.Endpoint, backup.AgentEndpointScheme)) {
		writeError(w, http.StatusConflict, "restore tests are "+backup.DrillNotApplicable)
		return
	}
	id, err := rt.backupProtection.StartDrill(r.Context(), latest.ID)
	if rt.writeDrillStartError(w, err) {
		return
	}
	if err != nil {
		rt.internalError(w, "api: restore test: start failed", err, slog.String("backup_id", latest.ID))
		return
	}
	writeJSON(w, http.StatusAccepted, restoreTestResource{ID: id, BackupID: latest.ID, Status: store.DrillStatusRunning, StartedAt: time.Now().UTC().Format(time.RFC3339)})
}

// handleGetDatabaseRestoreTest handles GET /api/v1/databases/{name}/restore-test:
// the newest restore test result, 404 when none has run yet.
func (rt *Router) handleGetDatabaseRestoreTest(w http.ResponseWriter, r *http.Request) {
	if rt.backupProtectionStore == nil {
		writeError(w, http.StatusNotImplemented, "restore tests are not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	drills, err := rt.backupProtectionStore.ListBackupDrills(r.Context(), "", "", name, 1)
	if err != nil {
		rt.internalError(w, "api: restore test: list failed", err, slog.String("name", name))
		return
	}
	if len(drills) == 0 {
		writeError(w, http.StatusNotFound, "no restore test has run for this database")
		return
	}
	d := drills[0]
	out := restoreTestResource{ID: d.ID, BackupID: d.BackupHistoryID, Status: d.Status, Stage: d.Stage, Bytes: d.Bytes, DurationMS: d.DurationMS, Error: d.Error, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt}
	writeJSON(w, http.StatusOK, out)
}

// restorePreviewResource is what a restore would do, read without changing anything.
type restorePreviewResource struct {
	Database   string `json:"database"`
	BackupID   string `json:"backup_id"`
	Engine     string `json:"engine"`
	SizeBytes  int64  `json:"size_bytes"`
	TakenAt    string `json:"taken_at"`
	AgeSeconds int64  `json:"age_seconds"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name,omitempty"`
	Encrypted  bool   `json:"encrypted"`
	// Overwrites names what the restore replaces: the live data of Database, in place.
	Overwrites string `json:"overwrites"`
	// SafetyBackupID is the newest other succeeded backup, the way back if this restore is wrong.
	SafetyBackupID string `json:"safety_backup_id,omitempty"`
	SafetyTakenAt  string `json:"safety_taken_at,omitempty"`
	// Busy is set while an import or restore already runs on the database.
	Busy bool `json:"busy"`
}

// handleDatabaseRestorePreview handles GET /api/v1/databases/{name}/restore-preview?backup_id=:
// size, age, target and overwrite scope of a restore, with the fallback backup named.
func (rt *Router) handleDatabaseRestorePreview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := rt.loadDatabaseForRunner(w, r, name, "api: restore preview: load database failed"); !ok {
		return
	}
	backupID := r.URL.Query().Get("backup_id")
	if backupID == "" {
		writeError(w, http.StatusBadRequest, "backup_id is required")
		return
	}
	bh, err := rt.backupHistory.GetBackupHistory(r.Context(), backupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) || (err == nil && bh.DatabaseName != name) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: restore preview: load backup failed", err, slog.String("backup_id", backupID))
		return
	}
	if bh.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, "only a succeeded backup can be restored from")
		return
	}
	db, _ := rt.databases.GetDesiredDatabase(r.Context(), name)
	out := restorePreviewResource{
		Database:   name,
		BackupID:   bh.ID,
		Engine:     db.Engine,
		SizeBytes:  bh.SizeBytes,
		TakenAt:    bh.StartedAt,
		TargetID:   bh.TargetID,
		Encrypted:  strings.Contains(bh.Codec, "age"),
		Overwrites: "all current data in database " + name,
		Busy:       backup.CheckNoImport(backup.DatabaseBusyKey(name)) != nil,
	}
	if t, terr := time.Parse(time.RFC3339, bh.StartedAt); terr == nil {
		out.AgeSeconds = int64(time.Since(t).Seconds())
	}
	if t, terr := rt.backupTargets.GetBackupTarget(r.Context(), bh.TargetID); terr == nil {
		out.TargetName = t.Name
	}
	if rows, lerr := rt.backupHistory.ListBackupHistory(r.Context(), name, restoreTestScanLimit, nil); lerr == nil {
		for i := range rows {
			if rows[i].Status == store.BackupStatusSucceeded && rows[i].ID != bh.ID {
				out.SafetyBackupID, out.SafetyTakenAt = rows[i].ID, rows[i].StartedAt
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}
