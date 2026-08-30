package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type logEntryResource struct {
	Timestamp  time.Time `json:"timestamp"`
	Stream     string    `json:"stream"`
	Message    string    `json:"message"`
	Structured bool      `json:"structured"`
	// FieldsJSON is Message's parsed JSON as a raw string when
	// Structured is true, empty otherwise, so a frontend log viewer
	// (TASKS.md 2.4) can render a structured line without re-parsing
	// Message itself. Passed through verbatim rather than re-encoded:
	// it's already valid JSON text (telemetry.LogEntry.FieldsJSON's own
	// contract), and json.Marshal would otherwise escape it as a string
	// instead of embedding it as a JSON value.
	FieldsJSON json.RawMessage `json:"fields,omitempty"`
}

type logsResponse struct {
	Entries []logEntryResource `json:"entries"`
}

// handleQueryLogs handles GET /api/v1/apps/{name}/logs; see
// queryResourceLogs for the shared implementation.
func (rt *Router) handleQueryLogs(w http.ResponseWriter, r *http.Request) {
	rt.queryResourceLogs(w, r, rt.lookupAppResource, "query logs", "app")
}

// queryResourceLogs is the shared body behind handleQueryLogs and
// handleQueryDatabaseLogs (database_logs.go): resolve name via lookup,
// then apply the same query params to both resource kinds (from/to
// RFC3339, same default window as handleQueryMetrics; q, a full-text
// search phrase, empty meaning every log line in range per
// telemetry.QueryLogs' own "empty query" contract). opName and noun feed
// the log lines and 404 message so an app 404 reads "app not found" and
// a database 404 reads "database not found".
func (rt *Router) queryResourceLogs(w http.ResponseWriter, r *http.Request, lookup resourceLookup, opName, noun string) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	resourceID, found, err := lookup(r.Context(), name)
	if !found && err == nil {
		writeError(w, http.StatusNotFound, noun+" not found")
		return
	} else if err != nil {
		rt.logger.Error("api: "+opName+": load "+noun+" failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	query := r.URL.Query().Get("q")

	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceID, from, to, query)
	if err != nil {
		if len(entries) == 0 {
			rt.logger.Error("api: "+opName+" failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		// Same partial-result handling as handleQueryMetrics: a
		// federated query with some data back is a warning, not a
		// failure, per ADR 008.
		rt.logger.Warn("api: "+opName+": partial result", slog.String("error", err.Error()), slog.String("name", name))
	}

	out := make([]logEntryResource, len(entries))
	for i, e := range entries {
		out[i] = toLogEntryResource(e)
	}
	writeJSON(w, http.StatusOK, logsResponse{Entries: out})
}

func toLogEntryResource(e telemetry.LogEntry) logEntryResource {
	r := logEntryResource{
		Timestamp:  e.Timestamp,
		Stream:     e.Stream,
		Message:    e.Message,
		Structured: e.Structured,
	}
	if e.Structured && e.FieldsJSON != "" {
		r.FieldsJSON = json.RawMessage(e.FieldsJSON)
	}
	return r
}
