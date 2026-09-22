package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/slowquery"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	defaultSlowQueryLimit = 100
	maxSlowQueryLimit     = 500
)

// slowQueryEntryResource is the wire shape for one parsed slow query.
type slowQueryEntryResource struct {
	Timestamp    time.Time `json:"timestamp"`
	DurationMs   float64   `json:"duration_ms"`
	Query        string    `json:"query"`
	RowsExamined int64     `json:"rows_examined,omitempty"`
}

type slowQueriesResponse struct {
	Entries []slowQueryEntryResource `json:"entries"`
	// Total is how many slow query entries were parsed in the requested
	// time range before Entries was truncated to limit/offset, so a
	// frontend can show "showing 100 of 342" rather than treating
	// len(Entries) as the whole count.
	Total int `json:"total"`
}

// handleQueryDatabaseSlowQueries handles GET
// /api/v1/databases/{name}/slow-queries. Only Postgres and MySQL are
// supported; every other engine returns 400 (see internal/slowquery's
// package doc comment for why). Postgres reads its already-stored
// container log lines (internal/slowquery), the same source
// database_logs.go uses. MySQL execs into the running container to read
// MySQLSlowQueryLogPath directly instead: MySQL 8's FILE log sink
// cannot reliably open /dev/stderr from inside a container (confirmed
// against a real container, see database.MySQLSlowQueryLogPath's own
// doc comment), so there is no Docker-log-stream source to read for
// this engine.
func (rt *Router) handleQueryDatabaseSlowQueries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	desired, err := rt.databases.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: query database slow queries: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, offset, ok := parseSlowQueryPageParams(w, r)
	if !ok {
		return
	}

	var parsed []slowquery.Entry
	switch desired.Engine {
	case store.EnginePostgres:
		parsed, ok = rt.postgresSlowQueryEntries(w, r, name, from, to)
	case store.EngineMySQL:
		parsed, ok = rt.mysqlSlowQueryEntries(w, r, name, desired.NodeID)
	default:
		writeError(w, http.StatusBadRequest, "slow query log is not supported for engine "+desired.Engine)
		return
	}
	if !ok {
		return
	}

	sort.SliceStable(parsed, func(i, j int) bool { return parsed[i].DurationMs > parsed[j].DurationMs })

	total := len(parsed)
	page := paginateSlowQueries(parsed, offset, limit)

	out := make([]slowQueryEntryResource, len(page))
	for i, e := range page {
		out[i] = slowQueryEntryResource{
			Timestamp:    e.Timestamp,
			DurationMs:   e.DurationMs,
			Query:        e.Query,
			RowsExamined: e.RowsExamined,
		}
	}
	writeJSON(w, http.StatusOK, slowQueriesResponse{Entries: out, Total: total})
}

// postgresSlowQueryEntries reads name's own stored container log lines
// (from-to bounded) and parses out slow-statement entries. Writes its
// own error response and reports false on failure.
func (rt *Router) postgresSlowQueryEntries(w http.ResponseWriter, r *http.Request, name string, from, to time.Time) ([]slowquery.Entry, bool) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return nil, false
	}

	logEntries, err := rt.telemetry.QueryLogs(r.Context(), resourceIDForDatabase(name), from, to, "")
	if err != nil {
		if len(logEntries) == 0 {
			rt.logger.Error("api: query database slow queries failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		// Same partial-result handling as queryResourceLogs/
		// queryResourceMetrics: a federated query with some data back is
		// a warning, not a failure, per ADR 008.
		rt.logger.Warn("api: query database slow queries: partial result", slog.String("error", err.Error()), slog.String("name", name))
	}

	return slowquery.ParsePostgres(toSlowQueryLines(logEntries)), true
}

const (
	mysqlSlowQueryExecTimeout = 10 * time.Second
	// mysqlSlowQueryTailLines bounds how much of the slow query log a
	// single request reads: generous relative to maxSlowQueryLimit given
	// a real entry is rarely more than ~6 lines.
	mysqlSlowQueryTailLines = 4000
)

// mysqlSlowQueryEntries execs into name's running container and reads
// MySQLSlowQueryLogPath directly (see this file's handler doc comment
// for why there is no log-stream source for MySQL). Writes its own
// error response and reports false on failure; a database with no
// slow queries logged yet (the file doesn't exist) is not an error, it
// reports true with a nil slice.
func (rt *Router) mysqlSlowQueryEntries(w http.ResponseWriter, r *http.Request, name, nodeID string) ([]slowquery.Entry, bool) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return nil, false
	}

	nodeRuntime, state, ok := rt.resolveDatabaseExecContainer(w, r, name, nodeID)
	if !ok {
		return nil, false
	}

	cmd := []string{"sh", "-c", fmt.Sprintf(`test -f %s && tail -n %d %s || true`, database.MySQLSlowQueryLogPath, mysqlSlowQueryTailLines, database.MySQLSlowQueryLogPath)}

	execCtx, cancel := context.WithTimeout(r.Context(), mysqlSlowQueryExecTimeout)
	defer cancel()

	rc, err := nodeRuntime.Exec(execCtx, state.ID, cmd)
	if err != nil {
		rt.logger.Error("api: query database slow queries: exec failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	defer func() { _ = rc.Close() }()

	capped := &cappedWriter{limit: execMaxOutputBytes}
	if _, err := io.Copy(capped, rc); err != nil {
		var execErr *docker.ExecExitError
		if !errors.As(err, &execErr) {
			rt.logger.Error("api: query database slow queries: read exec output failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		// A nonzero exit from this fixed "test -f && tail" command means
		// the container itself is in a bad state (shell/tail missing,
		// etc.), not "no slow queries yet" (that case is handled by the
		// "test -f" guard succeeding with no output, exit 0). Still not
		// worth failing the whole request over: report it and return
		// what, if anything, was captured.
		rt.logger.Warn("api: query database slow queries: exec exited nonzero", slog.String("name", name), slog.Int("exit_code", execErr.ExitCode), slog.String("stderr", execErr.Stderr))
	}

	return slowquery.ParseMySQL(mysqlLogLinesFrom(capped.buf.Bytes())), true
}

// resolveDatabaseExecContainer resolves the node runtime name is placed
// on (nodeID, desired.NodeID from the caller's already-loaded database)
// and its currently running container, the same
// resolve-runtime-then-inspect shape exec.go's resolveExecContainer
// uses for apps.
func (rt *Router) resolveDatabaseExecContainer(w http.ResponseWriter, r *http.Request, name, nodeID string) (docker.Runtime, *docker.ContainerState, bool) {
	nodeRuntime, err := rt.execRuntime(nodeID)
	if err != nil {
		rt.logger.Error("api: query database slow queries: resolve node runtime failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", nodeID))
		writeError(w, http.StatusBadGateway, "database's node is not currently reachable")
		return nil, nil, false
	}

	inspectCtx, cancel := context.WithTimeout(r.Context(), dockerInspectTimeout)
	defer cancel()
	state, err := nodeRuntime.InspectByName(inspectCtx, databaseContainerName(name))
	if err != nil {
		rt.logger.Error("api: query database slow queries: inspect container failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, nil, false
	}
	if state == nil || !state.Running {
		writeError(w, http.StatusConflict, "database has no running container")
		return nil, nil, false
	}
	return nodeRuntime, state, true
}

// mysqlLogLinesFrom splits raw exec output into LogLine values with a
// zero Timestamp: ParseMySQL derives each entry's real timestamp from
// its own "SET timestamp=..." line, not from LogLine.Timestamp, which
// this exec-based path has no per-line value for anyway (unlike the
// Docker-log-stream path Postgres uses).
func mysqlLogLinesFrom(raw []byte) []slowquery.LogLine {
	lines := bytes.Split(raw, []byte("\n"))
	out := make([]slowquery.LogLine, 0, len(lines))
	for _, l := range lines {
		if len(l) == 0 {
			continue
		}
		out = append(out, slowquery.LogLine{Message: string(l)})
	}
	return out
}

func toSlowQueryLines(entries []telemetry.LogEntry) []slowquery.LogLine {
	lines := make([]slowquery.LogLine, len(entries))
	for i, e := range entries {
		lines[i] = slowquery.LogLine{Timestamp: e.Timestamp, Message: e.Message}
	}
	return lines
}

func paginateSlowQueries(entries []slowquery.Entry, offset, limit int) []slowquery.Entry {
	if offset >= len(entries) {
		return nil
	}
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}
	return entries[offset:end]
}

// parseSlowQueryPageParams reads and validates the ?limit/?offset query
// parameters, the same "read, validate, write the 400 itself on failure"
// shape parseBackupHistoryListParams (backups.go) already establishes for
// a different endpoint's own pagination.
func parseSlowQueryPageParams(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	limit = defaultSlowQueryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return 0, 0, false
		}
		limit = n
	}
	if limit > maxSlowQueryLimit {
		limit = maxSlowQueryLimit
	}

	offset = 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "offset must not be negative")
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}
