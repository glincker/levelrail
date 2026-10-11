package api

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	envLogSignatureMaxRows     = "APP_LOG_SIGNATURE_MAX_ROWS"
	defaultLogSignatureMaxRows = 20000
	defaultSignatureTop        = 25
	maxSignatureTop            = 100
)

func logSignatureMaxRows() int {
	if n, err := strconv.Atoi(os.Getenv(envLogSignatureMaxRows)); err == nil && n > 0 {
		return n
	}
	return defaultLogSignatureMaxRows
}

type logSignaturesResponse struct {
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	Scanned     int       `json:"scanned"`
	MaxRows     int       `json:"max_rows"`
	Truncated   bool      `json:"truncated"`
	Total       int       `json:"total"`
	NewCount    int       `json:"new_count"`
	HasBaseline bool      `json:"has_baseline"`

	Signatures []telemetry.Signature `json:"signatures"`
}

// handleLogSignatures handles GET /api/v1/apps/{name}/logs/signatures: the
// top clusters of similar error lines in a window. With baseline=true the
// window of equal length just before it is clustered too and signatures
// absent there are flagged new.
func (rt *Router) handleLogSignatures(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	resourceID, found, err := rt.lookupAppResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: log signatures: load app", err, slog.String("name", name))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	minLevel := telemetry.LevelError
	if raw := r.URL.Query().Get("level"); raw != "" {
		if minLevel, err = parseLevelParam(raw); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	top := defaultSignatureTop
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxSignatureTop {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(maxSignatureTop))
			return
		}
		top = n
	}
	q := r.URL.Query().Get("q")
	maxRows := logSignatureMaxRows()

	entries, truncated, err := telemetry.QueryLogsBounded(r.Context(), rt.telemetry, resourceID, from, to, q, maxRows)
	if err != nil && len(entries) == 0 {
		rt.internalError(w, "api: log signatures: query", err, slog.String("name", name))
		return
	}
	opts := telemetry.SignatureOptions{MinLevel: minLevel, Buckets: 24}
	all := telemetry.AggregateSignatures(entries, from, to, opts)
	res := logSignaturesResponse{From: from, To: to, Scanned: len(entries), MaxRows: maxRows, Truncated: truncated, Total: len(all)}

	if r.URL.Query().Get("baseline") == "true" {
		span := to.Sub(from)
		prev, _, err := telemetry.QueryLogsBounded(r.Context(), rt.telemetry, resourceID, from.Add(-span), from, q, maxRows)
		if err != nil && len(prev) == 0 {
			rt.logger.Warn("api: log signatures: baseline query failed", slog.String("name", name), slog.String("error", err.Error()))
		} else {
			res.HasBaseline = true
			res.NewCount = telemetry.MarkNewSignatures(all, telemetry.AggregateSignatures(prev, from.Add(-span), from, opts))
		}
	}
	if len(all) > top {
		all = all[:top]
	}
	res.Signatures = all
	writeJSON(w, http.StatusOK, res)
}

// logsHaveSignature keeps only entries whose message clusters under id.
func logsHaveSignature(entries []telemetry.LogEntry, id string) []telemetry.LogEntry {
	out := entries[:0:0]
	for _, e := range entries {
		if got, _ := telemetry.SignatureOf(e); got == id {
			out = append(out, e)
		}
	}
	return out
}
