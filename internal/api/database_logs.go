package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// resourceIDForDatabase is telemetry's stable identifier for one managed
// database's metrics/logs, the database-kind counterpart to
// resourceIDForApp (metrics.go). Must match what cmd/levelrail/main.go's
// telemetryTargets/logTargets write samples and log entries under, the
// same rename hazard resourceIDForApp's own doc comment calls out.
func resourceIDForDatabase(name string) string {
	return "database:" + name
}

// handleQueryDatabaseLogs handles GET /api/v1/databases/{name}/logs: the
// database-kind counterpart to handleQueryLogs (logs.go), same query
// params (from/to/q) and response shape, differing only in which store
// lookup confirms the resource exists and which resourceID key is
// queried.
func (rt *Router) handleQueryDatabaseLogs(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: query database logs: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	query := r.URL.Query().Get("q")

	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceIDForDatabase(name), from, to, query)
	if err != nil {
		if len(entries) == 0 {
			rt.logger.Error("api: query database logs failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		rt.logger.Warn("api: query database logs: partial result", slog.String("error", err.Error()), slog.String("name", name))
	}

	out := make([]logEntryResource, len(entries))
	for i, e := range entries {
		out[i] = toLogEntryResource(e)
	}
	writeJSON(w, http.StatusOK, logsResponse{Entries: out})
}

// handleLiveDatabaseLogStream handles
// GET /api/v1/databases/{name}/logs/stream: the database-kind
// counterpart to handleLiveLogStream (live_logs.go). Same
// backfill-then-subscribe handoff, same 501 gating on
// telemetry/logBroadcaster, differing only in the store lookup and
// resourceID key; see handleLiveLogStream's own doc comment for the
// ordering guarantee this mirrors.
func (rt *Router) handleLiveDatabaseLogStream(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil || rt.logBroadcaster == nil {
		writeError(w, http.StatusNotImplemented, "live log streaming is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: live database log stream: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resourceID := resourceIDForDatabase(name)

	live, unsubscribe := rt.logBroadcaster.Subscribe(resourceID)
	defer unsubscribe()
	subscribeTime := time.Now()

	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceID, subscribeTime.Add(-liveLogBackfillWindow), subscribeTime, "")
	if err != nil {
		rt.logger.Error("api: live database log stream: backfill query failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: live database log stream: response writer does not support flushing", slog.String("name", name))
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for _, e := range trimBackfill(entries, subscribeTime, liveLogBackfillMaxLines) {
		writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, chOpen := <-live:
			if !chOpen {
				return
			}
			writeSSEEvent(w, sseLogEvent{Line: entry.Message, Stream: entry.Stream})
			flusher.Flush()
		}
	}
}
