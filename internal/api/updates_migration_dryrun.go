package api

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

// migrationDryRunBusy lets one dry run work at a time; a copy of a large
// database per request would otherwise fill the data volume.
var migrationDryRunBusy = make(chan struct{}, 1)

// handleMigrationDryRun handles POST /api/v1/updates/migration-dry-run: it
// applies this binary's pending migrations to a scratch copy of the control
// plane database and reports the result. The live database is never written.
func (rt *Router) handleMigrationDryRun(w http.ResponseWriter, r *http.Request) {
	if rt.dataDir == "" {
		writeError(w, http.StatusNotImplemented, "migration dry run needs a data directory")
		return
	}
	select {
	case migrationDryRunBusy <- struct{}{}:
		defer func() { <-migrationDryRunBusy }()
	default:
		writeError(w, http.StatusTooManyRequests, "a migration dry run is already running")
		return
	}
	res, err := selfupgrade.MigrationDryRun(r.Context(), selfupgrade.MigrationDryRunOptions{
		LiveDB:     filepath.Join(rt.dataDir, liveDBFile),
		ScratchDir: rt.dataDir,
		FreeBytes:  rt.dataDirFreeBytes(),
	})
	if errors.Is(err, selfupgrade.ErrDryRunDiskShort) {
		writeError(w, http.StatusInsufficientStorage, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: migration dry run failed", err)
		return
	}
	detail := "migration dry run ok"
	if !res.OK {
		detail = "migration dry run failed: " + res.Error
	}
	rt.auditUpgradeEvent(r.Context(), r, "upgrade.migration_dry_run", detail)
	writeJSON(w, http.StatusOK, res)
}
