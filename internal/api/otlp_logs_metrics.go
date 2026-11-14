package api

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/otlpingest"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// otlpSink is what the logs and metrics receiver writes into. The telemetry
// database that backs error tracking also satisfies it.
type otlpSink interface {
	WriteLogBatch(ctx context.Context, entries []telemetry.LogEntry) error
	WriteSamples(ctx context.Context, samples []telemetry.Sample) error
}

func (rt *Router) otlpSink() otlpSink {
	if s, ok := rt.errTrack.store.(otlpSink); ok {
		return s
	}
	if s, ok := rt.traces.(otlpSink); ok {
		return s
	}
	return nil
}

func (rt *Router) handleOTLPLogs(w http.ResponseWriter, r *http.Request) {
	rt.ingestOTLP(w, r, true)
}

func (rt *Router) handleOTLPMetrics(w http.ResponseWriter, r *http.Request) {
	rt.ingestOTLP(w, r, false)
}

// errCodeFeatureDisabled marks a 501 that an operator can fix by turning a setting on.
const errCodeFeatureDisabled = "feature_disabled"

// ingestOTLP handles POST /api/v1/otlp/v1/{logs,metrics}. It reuses the
// per-app tracing key, is off unless the runtime setting is on, and maps the
// payload into the existing log and metric stores.
func (rt *Router) ingestOTLP(w http.ResponseWriter, r *http.Request, logs bool) {
	set := rt.changesSettings()
	sink := rt.otlpSink()
	if sink == nil || !set.Bool(r.Context(), runtimesettings.KeyOTLPIngestEnabled) {
		writeJSON(w, http.StatusNotImplemented, apiError{Error: "OTLP logs and metrics ingest is not enabled on this control plane", Code: errCodeFeatureDisabled, RequestID: w.Header().Get(requestIDHeader)})
		return
	}
	app := rt.authTraceIngest(r, r.Header.Get("Authorization"))
	if app == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid ingest key")
		return
	}
	limit := int64(set.Limit(r.Context(), runtimesettings.KeyOTLPIngestMaxBody))
	var src io.Reader = http.MaxBytesReader(w, r.Body, limit)
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(src)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid gzip body")
			return
		}
		defer func() { _ = zr.Close() }()
		src = io.LimitReader(zr, limit+1)
	}
	body, err := io.ReadAll(src)
	if err != nil || int64(len(body)) > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	maxRecords := set.Limit(r.Context(), runtimesettings.KeyOTLPIngestMaxRecords)
	ct := r.Header.Get("Content-Type")
	resource := "service:" + app
	if logs {
		entries, derr := otlpingest.DecodeLogs(ct, body, resource, maxRecords, set.Limit(r.Context(), runtimesettings.KeyOTLPIngestMaxLine))
		if err = rt.otlpFinish(w, derr); err == nil {
			err = sink.WriteLogBatch(r.Context(), entries)
		}
	} else {
		samples, derr := otlpingest.DecodeMetrics(ct, body, resource, maxRecords)
		if err = rt.otlpFinish(w, derr); err == nil {
			err = sink.WriteSamples(r.Context(), samples)
		}
	}
	if errors.Is(err, errOTLPHandled) {
		return
	}
	if err != nil {
		rt.internalError(w, "api: write otlp telemetry", err, slog.String("app", app))
		return
	}
	if strings.HasPrefix(ct, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

var errOTLPHandled = errors.New("otlp: response written")

// otlpFinish answers a decode failure and reports errOTLPHandled so the caller stops.
func (rt *Router) otlpFinish(w http.ResponseWriter, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, otlpingest.ErrUnsupportedType):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, otlpingest.ErrTooMany):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
	return errOTLPHandled
}
