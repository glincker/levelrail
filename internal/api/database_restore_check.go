package api

import (
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
