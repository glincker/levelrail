package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxDataCopyBody = 16 << 10
	envCopyTimeout  = "APP_MIGRATE_COPY_TIMEOUT"
	defaultCopyTime = 2 * time.Hour
)

// DataImportStore is the store surface the live data copy routes need.
type DataImportStore interface {
	ClaimDatabaseDataImport(ctx context.Context, name, host string, port int, srcDB string, now time.Time, staleAfter time.Duration) error
	FinishDatabaseDataImport(ctx context.Context, name, status, reason string, checked, mismatched int, detail string, now time.Time) error
	GetDatabaseDataImport(ctx context.Context, name string) (store.DatabaseDataImport, bool, error)
	ListDatabaseDataImports(ctx context.Context) ([]store.DatabaseDataImport, error)
}

type dataImportResource struct {
	Database       string                   `json:"database"`
	Engine         string                   `json:"engine"`
	Status         string                   `json:"status"`
	Reason         string                   `json:"reason,omitempty"`
	NextAction     string                   `json:"next_action,omitempty"`
	SourceHost     string                   `json:"source_host,omitempty"`
	SourcePort     int                      `json:"source_port,omitempty"`
	SourceDatabase string                   `json:"source_database,omitempty"`
	Checked        int                      `json:"checked"`
	Mismatched     int                      `json:"mismatched"`
	Tables         []datamigrate.TableCount `json:"tables,omitempty"`
	StartedAt      string                   `json:"started_at,omitempty"`
	FinishedAt     string                   `json:"finished_at,omitempty"`
}

func copyTimeout() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envCopyTimeout)); err == nil && d > 0 {
		return d
	}
	return defaultCopyTime
}

func toDataImportResource(db store.DesiredDatabase, row store.DatabaseDataImport, found bool) dataImportResource {
	res := dataImportResource{Database: db.Name, Engine: db.Engine, Status: datamigrate.StatusPending}
	switch {
	case !datamigrate.Supported(db.Engine):
		res.Status = datamigrate.StatusUnsupported
		res.NextAction = "Automatic copy covers postgres, mysql, mariadb, mongodb and redis. Restore this engine from a dump of the source with the backup and restore tools."
	case !found:
		res.NextAction = "Provide the source host and credentials to copy the data in."
	}
	if !found {
		return res
	}
	res.Status, res.Reason = row.Status, row.Reason
	res.SourceHost, res.SourcePort, res.SourceDatabase = row.SourceHost, row.SourcePort, row.SourceDB
	res.Checked, res.Mismatched = row.Checked, row.Mismatched
	if row.Detail != "" {
		var v datamigrate.Verification
		if json.Unmarshal([]byte(row.Detail), &v) == nil {
			res.Tables = v.Tables
		}
	}
	if !row.StartedAt.IsZero() {
		res.StartedAt = row.StartedAt.Format(time.RFC3339)
	}
	if !row.FinishedAt.IsZero() {
		res.FinishedAt = row.FinishedAt.Format(time.RFC3339)
	}
	switch row.Status {
	case store.DataImportFailed:
		res.NextAction = "Fix the cause above and run the copy again. It replaces the target's contents, so repeating it is safe."
	case store.DataImportVerified:
		res.NextAction = "Point the app's connection settings at this database, then run the cutover check."
	}
	return res
}

// handleCopyDatabaseData handles POST /api/v1/imports/platform/databases/{name}/copy.
// The source password lives in the request and the helper container only.
func (rt *Router) handleCopyDatabaseData(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: copy database data: load database failed")
	if !ok {
		return
	}
	var src datamigrate.Source
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDataCopyBody)).Decode(&src); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := src.Validate(db.Engine); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "copying data is not available on this control plane")
		return
	}
	runtime, err := rt.execRuntime(db.NodeID)
	if err != nil {
		rt.logger.Error("api: copy database data: resolve node runtime failed", slog.String("error", err.Error()), slog.String("database", name), slog.String("node_id", db.NodeID))
		writeError(w, http.StatusBadGateway, "the database's node is not currently reachable")
		return
	}
	timeout := copyTimeout()
	err = rt.dataImports.ClaimDatabaseDataImport(r.Context(), name, src.Host, src.Port, src.Database, time.Now(), timeout)
	if errors.Is(err, store.ErrDataImportInProgress) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: copy database data: claim failed", err, slog.String("database", name))
		return
	}

	copier := &datamigrate.Copier{Runtime: runtime, Restorer: &backup.ContainerRestorer{Runtime: runtime}, Logger: rt.logger}
	target := datamigrate.Target{Name: name, Engine: db.Engine, Version: db.Version}
	go rt.runDataCopy(copier, target, src, timeout) //nolint:gosec // detached on purpose: r.Context() is cancelled when this handler returns

	row, found, _ := rt.dataImports.GetDatabaseDataImport(r.Context(), name)
	writeJSON(w, http.StatusAccepted, toDataImportResource(*db, row, found))
}

// runDataCopy runs detached from the request, which is cancelled the moment
// the handler returns.
func (rt *Router) runDataCopy(c *datamigrate.Copier, t datamigrate.Target, src datamigrate.Source, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	v, err := c.Copy(ctx, t, src)
	status, reason := store.DataImportVerified, ""
	if err != nil {
		status, reason = store.DataImportFailed, err.Error()
		rt.logger.Warn("api: database data copy failed", slog.String("database", t.Name), slog.String("reason", reason))
	} else {
		rt.logger.Info("api: database data copy verified", slog.String("database", t.Name), slog.Int("tables", v.Checked))
	}
	detail, _ := json.Marshal(v)
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	if ferr := rt.dataImports.FinishDatabaseDataImport(finishCtx, t.Name, status, reason, v.Checked, v.Mismatched, string(detail), time.Now()); ferr != nil {
		rt.logger.Error("api: record database data copy outcome failed", slog.String("error", ferr.Error()), slog.String("database", t.Name))
	}
}

// handleGetDatabaseDataCopy handles GET /api/v1/imports/platform/databases/{name}.
func (rt *Router) handleGetDatabaseDataCopy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: get database data copy: load database failed")
	if !ok {
		return
	}
	row, found, err := rt.dataImports.GetDatabaseDataImport(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get database data copy failed", err, slog.String("database", name))
		return
	}
	writeJSON(w, http.StatusOK, toDataImportResource(*db, row, found))
}

// handleListDatabaseDataCopies handles GET /api/v1/imports/platform/databases.
func (rt *Router) handleListDatabaseDataCopies(w http.ResponseWriter, r *http.Request) {
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		rt.internalError(w, "api: list database data copies: list databases failed", err)
		return
	}
	rows, err := rt.dataImports.ListDatabaseDataImports(r.Context())
	if err != nil {
		rt.internalError(w, "api: list database data copies failed", err)
		return
	}
	byName := make(map[string]store.DatabaseDataImport, len(rows))
	for _, row := range rows {
		byName[row.DatabaseName] = row
	}
	out := make([]dataImportResource, 0, len(dbs))
	for _, db := range dbs {
		row, found := byName[db.Name]
		out = append(out, toDataImportResource(db, row, found))
	}
	writeJSON(w, http.StatusOK, out)
}
