package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
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
	// can render a structured line without re-parsing
	// Message itself. Passed through verbatim rather than re-encoded:
	// it's already valid JSON text (telemetry.LogEntry.FieldsJSON's own
	// contract), and json.Marshal would otherwise escape it as a string
	// instead of embedding it as a JSON value.
	FieldsJSON json.RawMessage `json:"fields,omitempty"`
	// Level is the detected log level, empty when the line carries none.
	Level string `json:"level,omitempty"`
	// Container is the id of the container that wrote the line.
	Container string `json:"container,omitempty"`
}

// logContainer is one container seen in a log query, with its line count.
type logContainer struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// logsResponse is the log query result. Total counts the entries that
// matched the time window, q and level before limit trimmed them.
type logsResponse struct {
	Entries []logEntryResource `json:"entries"`
	Total   int                `json:"total"`
	// Containers lists every container that matched before the container
	// filter applied, so a viewer can offer the choices.
	Containers []logContainer `json:"containers"`
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
// telemetry.QueryLogs' own "empty query" contract; level, a minimum
// level; limit, keep only the newest N matches). opName and noun feed
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
	minLevel, err := parseLevelParam(r.URL.Query().Get("level"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseLimitParam(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fieldFilters, err := telemetry.ParseFieldFilters(r.URL.Query()["field"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stream := r.URL.Query().Get("stream")
	if stream != "" && stream != "stdout" && stream != "stderr" {
		writeError(w, http.StatusBadRequest, "stream must be stdout or stderr")
		return
	}
	container := r.URL.Query().Get("container")

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

	entries = filterLogsByLevel(entries, minLevel)
	entries = filterLogs(entries, stream, fieldFilters)
	containers := countLogContainers(entries)
	if container != "" {
		entries = filterLogsByContainer(entries, container)
	}
	total := len(entries)
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	out := make([]logEntryResource, len(entries))
	for i, e := range entries {
		out[i] = toLogEntryResource(e)
	}
	writeJSON(w, http.StatusOK, logsResponse{Entries: out, Total: total, Containers: containers})
}

func toLogEntryResource(e telemetry.LogEntry) logEntryResource {
	r := logEntryResource{
		Timestamp:  e.Timestamp,
		Stream:     e.Stream,
		Message:    e.Message,
		Structured: e.Structured,
		Level:      classifyLogLevel(e),
		Container:  e.ContainerID,
	}
	if e.Structured && e.FieldsJSON != "" {
		r.FieldsJSON = json.RawMessage(e.FieldsJSON)
	}
	return r
}

func filterLogs(entries []telemetry.LogEntry, stream string, filters []telemetry.FieldFilter) []telemetry.LogEntry {
	if stream == "" && len(filters) == 0 {
		return entries
	}
	out := entries[:0:0]
	for _, e := range entries {
		if stream != "" && e.Stream != stream {
			continue
		}
		if !telemetry.MatchFieldFilters(e, filters) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func filterLogsByContainer(entries []telemetry.LogEntry, prefix string) []telemetry.LogEntry {
	out := entries[:0:0]
	for _, e := range entries {
		if strings.HasPrefix(e.ContainerID, prefix) {
			out = append(out, e)
		}
	}
	return out
}

func countLogContainers(entries []telemetry.LogEntry) []logContainer {
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.ContainerID]++
	}
	out := make([]logContainer, 0, len(counts))
	for id, n := range counts {
		out = append(out, logContainer{ID: id, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].ID < out[j].ID
	})
	return out
}
