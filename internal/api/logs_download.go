package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// logDownloadMaxLines caps a single download to this many of the most
// recent lines in range. Higher than liveLogBackfillMaxLines/
// crashloopLogLines (both 200, sized for an at-a-glance UI panel): this
// is a deliberate file export for a support ticket or archive, so a
// larger but still bounded cap keeps it a real download rather than an
// unbounded dump of the whole store.
const defaultLogDownloadMaxLines = 5000

// logDownloadMaxLines is the cap in effect, tunable with APP_LOG_EXPORT_MAX_LINES.
func logDownloadMaxLines() int {
	if n, err := strconv.Atoi(os.Getenv("APP_LOG_EXPORT_MAX_LINES")); err == nil && n > 0 {
		return n
	}
	return defaultLogDownloadMaxLines
}

// handleDownloadLogs handles GET /api/v1/apps/{name}/logs/download; see
// downloadResourceLogs for the shared implementation.
func (rt *Router) handleDownloadLogs(w http.ResponseWriter, r *http.Request) {
	rt.downloadResourceLogs(w, r, rt.lookupAppResource, "download logs", "app")
}

// downloadResourceLogs answers the same from/to/q query used by
// queryResourceLogs, through the same telemetry.QueryLogs call, but
// writes the result as a plain-text attachment instead of a JSON body,
// trimmed to logDownloadMaxLines most-recent lines the same way the CLI's
// --tail already trims (cmd/levelrail-cli/apps_logs.go's tailEntries).
func (rt *Router) downloadResourceLogs(w http.ResponseWriter, r *http.Request, lookup resourceLookup, opName, noun string) {
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
		rt.internalError(w, "api: "+opName+": load "+noun+" failed", err, slog.String("name", name))
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	query := r.URL.Query().Get("q")
	if err := telemetry.ValidateLogSearch(query); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	minLevel, err := parseLevelParam(r.URL.Query().Get("level"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fieldFilters, err := telemetry.ParseFieldFilters(r.URL.Query()["field"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceID, from, to, query)
	if err != nil {
		if len(entries) == 0 {
			rt.internalError(w, "api: "+opName+" failed", err, slog.String("name", name))
			return
		}
		rt.logger.Warn("api: "+opName+": partial result", slog.String("error", err.Error()), slog.String("name", name))
	}
	if sig := r.URL.Query().Get("signature"); sig != "" {
		entries = logsHaveSignature(entries, sig)
	}
	entries = filterLogsByLevel(entries, minLevel)
	entries = filterLogs(entries, r.URL.Query().Get("stream"), fieldFilters)
	if c := r.URL.Query().Get("container"); c != "" {
		entries = filterLogsByContainer(entries, c)
	}
	if limit := logDownloadMaxLines(); len(entries) > limit {
		entries = entries[len(entries)-limit:]
		w.Header().Set("X-Log-Export-Truncated", "true")
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", logDownloadFilename(noun, name, to)))
	w.WriteHeader(http.StatusOK)
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%s %s %s\n", e.Timestamp.UTC().Format(time.RFC3339), e.Stream, telemetry.RedactLine(e.Message))
	}
}

// logDownloadFilename derives a browser-facing filename from noun (e.g.
// "app"), name, and the download's own "to" bound, sanitizing name so a
// slash or space in it can't turn into an unexpected path segment or a
// visually broken Content-Disposition value.
func logDownloadFilename(noun, name string, to time.Time) string {
	safeName := strings.NewReplacer("/", "-", " ", "-").Replace(name)
	return fmt.Sprintf("%s-%s-logs-%s.txt", noun, safeName, to.UTC().Format("20060102T150405Z"))
}
