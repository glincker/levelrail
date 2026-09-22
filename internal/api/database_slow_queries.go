package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

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
// /api/v1/databases/{name}/slow-queries, parsing this database's slow
// query log out of its already-stored container log lines (internal/
// slowquery). Only Postgres and MySQL are supported; every other engine
// returns 400 (see internal/slowquery's package doc comment for why).
func (rt *Router) handleQueryDatabaseSlowQueries(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

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

	var parser func([]slowquery.LogLine) []slowquery.Entry
	switch desired.Engine {
	case store.EnginePostgres:
		parser = slowquery.ParsePostgres
	case store.EngineMySQL:
		parser = slowquery.ParseMySQL
	default:
		writeError(w, http.StatusBadRequest, "slow query log is not supported for engine "+desired.Engine)
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

	logEntries, err := rt.telemetry.QueryLogs(ctx, resourceIDForDatabase(name), from, to, "")
	if err != nil {
		if len(logEntries) == 0 {
			rt.logger.Error("api: query database slow queries failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		// Same partial-result handling as queryResourceLogs/
		// queryResourceMetrics: a federated query with some data back is
		// a warning, not a failure, per ADR 008.
		rt.logger.Warn("api: query database slow queries: partial result", slog.String("error", err.Error()), slog.String("name", name))
	}

	parsed := parser(toSlowQueryLines(logEntries))
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
