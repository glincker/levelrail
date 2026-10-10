package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

// viewerTarget resolves a database's running container into a console
// target. Credentials stay in the container: the control plane only execs
// the engine's own client there. Writes the error response and reports
// false on failure.
func (rt *Router) viewerTarget(w http.ResponseWriter, r *http.Request, wantKV bool) (dbviewer.Target, *store.DesiredDatabase, bool) {
	name := r.PathValue("name")
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		if t, d, found, ok := rt.externalViewerTarget(w, r, name, wantKV); found {
			return t, d, ok
		}
		writeError(w, http.StatusNotFound, "database not found")
		return dbviewer.Target{}, desired, false
	}
	if err != nil {
		rt.internalError(w, "api: database viewer: load database failed", err, slog.String("name", name))
		return dbviewer.Target{}, desired, false
	}

	var dialect dbviewer.Dialect
	switch desired.Engine {
	case store.EnginePostgres:
		dialect = dbviewer.DialectPostgres
	case store.EngineMySQL:
		dialect = dbviewer.DialectMySQL
	case store.EngineMariaDB:
		dialect = dbviewer.DialectMariaDB
	case store.EngineRedis, store.EngineKeyDB, store.EngineDragonfly:
		if !wantKV {
			writeError(w, http.StatusBadRequest, "SQL console is not supported for engine "+desired.Engine)
			return dbviewer.Target{}, desired, false
		}
	default:
		writeError(w, http.StatusBadRequest, "database viewer is not supported for engine "+desired.Engine)
		return dbviewer.Target{}, desired, false
	}
	if wantKV && dialect != "" {
		writeError(w, http.StatusBadRequest, "key browser is not supported for engine "+desired.Engine)
		return dbviewer.Target{}, desired, false
	}

	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return dbviewer.Target{}, desired, false
	}
	nodeRuntime, state, ok := rt.resolveDatabaseExecContainer(w, r, name, desired.NodeID)
	if !ok {
		return dbviewer.Target{}, desired, false
	}
	return dbviewer.Target{
		Exec:        nodeRuntime,
		ContainerID: state.ID,
		Dialect:     dialect,
		Limits:      dbviewer.LimitsFromEnv(nil),
	}, desired, true
}

// writeViewerError maps a dbviewer failure to a response. Database error
// text is shown to the caller; everything else is logged and generic.
func (rt *Router) writeViewerError(w http.ResponseWriter, context, name string, err error) {
	var qe *dbviewer.QueryError
	switch {
	case errors.As(err, &qe):
		writeError(w, http.StatusBadRequest, qe.Message)
	case errors.Is(err, dbviewer.ErrTimeout):
		writeError(w, http.StatusRequestTimeout, err.Error())
	case errors.Is(err, dbviewer.ErrNotFound):
		writeError(w, http.StatusNotFound, "table, column, or key not found")
	case errors.Is(err, dbviewer.ErrBackslash), errors.Is(err, dbviewer.ErrNotAllowed):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		rt.internalError(w, context, err, slog.String("name", name))
	}
}

type databaseSchemaResponse struct {
	Engine  string                `json:"engine"`
	Schemas []dbviewer.SchemaNode `json:"schemas"`
}

// handleGetDatabaseSchema handles GET /api/v1/databases/{name}/schema.
func (rt *Router) handleGetDatabaseSchema(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, false)
	if !ok {
		return
	}
	nodes, err := target.Schema(r.Context())
	if err != nil {
		rt.writeViewerError(w, "api: database schema failed", desired.Name, err)
		return
	}
	writeJSON(w, http.StatusOK, databaseSchemaResponse{Engine: desired.Engine, Schemas: nodes})
}

// handleGetDatabaseTableRows handles GET
// /api/v1/databases/{name}/tables/{schema}/{table}/rows.
func (rt *Router) handleGetDatabaseTableRows(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, false)
	if !ok {
		return
	}
	q := r.URL.Query()
	pq := dbviewer.PageQuery{
		Schema:       r.PathValue("schema"),
		Table:        r.PathValue("table"),
		SortColumn:   q.Get("sort"),
		SortDesc:     q.Get("dir") == "desc",
		FilterColumn: q.Get("filter_column"),
		FilterOp:     q.Get("filter_op"),
		FilterValue:  q.Get("filter_value"),
	}
	var err error
	if raw := q.Get("limit"); raw != "" {
		if pq.Limit, err = strconv.Atoi(raw); err != nil || pq.Limit < 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
	}
	if raw := q.Get("offset"); raw != "" {
		if pq.Offset, err = strconv.Atoi(raw); err != nil || pq.Offset < 0 {
			writeError(w, http.StatusBadRequest, "offset must not be negative")
			return
		}
	}
	page, err := target.TablePage(r.Context(), pq)
	if err != nil {
		rt.writeViewerError(w, "api: database table rows failed", desired.Name, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleScanDatabaseKeys handles GET /api/v1/databases/{name}/keys
// (Redis-family engines): one read-only SCAN step.
func (rt *Router) handleScanDatabaseKeys(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, true)
	if !ok {
		return
	}
	q := r.URL.Query()
	count := 0
	if raw := q.Get("count"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "count must be a positive integer")
			return
		}
		count = n
	}
	scan, err := target.ScanKeys(r.Context(), q.Get("cursor"), q.Get("pattern"), count)
	if err != nil {
		rt.writeViewerError(w, "api: database key scan failed", desired.Name, err)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

// handleGetDatabaseKey handles GET /api/v1/databases/{name}/key?key=.
func (rt *Router) handleGetDatabaseKey(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, true)
	if !ok {
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	val, err := target.GetKey(r.Context(), key)
	if err != nil {
		rt.writeViewerError(w, "api: database key read failed", desired.Name, err)
		return
	}
	writeJSON(w, http.StatusOK, val)
}
