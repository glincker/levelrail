package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
)

// handleScrapeMetrics serves the Prometheus text exposition at GET /metrics.
// It is gated like every read route: configure Prometheus with a read token
// as authorization.credentials. A caller restricted to some apps only sees
// those apps; databases follow the same rule and nodes are shown only to an
// unrestricted caller. Labels are limited to app, database, node and the
// exporter's own destination name, so cardinality tracks what is deployed.
func (rt *Router) handleScrapeMetrics(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	_, _, scope, err := rt.callerScope(r)
	if err != nil {
		rt.internalError(w, "api: scrape metrics: resolve caller scope", err)
		return
	}
	allow := func(kind, name string) bool {
		switch kind {
		case "app":
			return scope.visible(resourcePrefixApp, name)
		case "database":
			return scope.visible(resourcePrefixDatabase, name)
		}
		return !scope.restricted()
	}
	series, err := svc.Series(r.Context(), allow)
	if err != nil {
		rt.logger.Warn("api: scrape metrics: partial result", slog.String("error", err.Error()))
	}
	if scope.restricted() {
		series = dropPlatformSeries(series)
	}
	var buf bytes.Buffer
	if err := dataexport.WritePrometheus(&buf, series); err != nil {
		rt.internalError(w, "api: scrape metrics: render", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

var resourceLabels = map[string]bool{"app": true, "database": true, "node": true}

// dropPlatformSeries removes series that carry no resource label (platform
// gauges and exporter self metrics), which a restricted caller must not see.
func dropPlatformSeries(in []dataexport.Series) []dataexport.Series {
	out := in[:0:0]
	for _, s := range in {
		if len(s.Labels) == 1 && resourceLabels[s.Labels[0].Name] {
			out = append(out, s)
		}
	}
	return out
}

var exportAuditCSVHeader = []string{
	"id", "time", "actor_type", "actor_id", "actor_name", "ability", "method", "path",
	"status", "remote_addr", "client_kind", "agent_name", "agent_client", "action",
}

// handleExportAuditLog streams the audit log between ?from (inclusive) and
// ?to (exclusive), oldest first, as NDJSON (default) or ?format=csv. Rows
// pass through the same redaction as the streaming export.
func (rt *Router) handleExportAuditLog(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.dataExportOrNotImplemented(w)
	if !ok {
		return
	}
	var from, to time.Time
	for _, p := range []struct {
		name string
		dst  *time.Time
	}{{"from", &from}, {"to", &to}} {
		if raw := r.URL.Query().Get(p.name); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, p.name+" must be an RFC3339 timestamp")
				return
			}
			*p.dst = t
		}
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "ndjson"
	}
	if format != "ndjson" && format != "csv" {
		writeError(w, http.StatusBadRequest, `format must be "ndjson" or "csv"`)
		return
	}

	var emit func(dataexport.AuditRecord) error
	var finish func() error
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-log.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write(exportAuditCSVHeader)
		emit = func(a dataexport.AuditRecord) error {
			return cw.Write([]string{
				a.ID, a.Time, csvSafe(a.ActorType), csvSafe(a.ActorID), csvSafe(a.ActorName), csvSafe(a.Ability),
				csvSafe(a.Method), csvSafe(a.Path), strconv.Itoa(a.Status), a.RemoteAddr, a.ClientKind,
				csvSafe(a.AgentName), csvSafe(a.AgentClient), csvSafe(a.Action),
			})
		}
		finish = func() error { cw.Flush(); return cw.Error() }
	} else {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-log.ndjson"`)
		enc := json.NewEncoder(w)
		emit = func(a dataexport.AuditRecord) error { return enc.Encode(a) }
		finish = func() error { return nil }
	}
	w.WriteHeader(http.StatusOK)
	if err := svc.StreamAudit(r.Context(), from, to, emit); err != nil {
		rt.logger.Warn("api: export audit log failed mid-stream", slog.String("error", err.Error()))
	}
	if err := finish(); err != nil {
		rt.logger.Warn("api: export audit log flush failed", slog.String("error", err.Error()))
	}
}
