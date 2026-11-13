package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	logscol "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricscol "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestOTLPLogsAndMetricsIngest(t *testing.T) {
	rt, db, tel := newTraceRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tracing", ""))
	key := decodeTracingKey(t, rec)

	logBody, _ := proto.Marshal(&logscol.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{
		{TimeUnixNano: uint64(time.Now().UnixNano()), SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
			Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "disk full"}}},
	}}}}}})
	metBody, _ := proto.Marshal(&metricscol.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{
		{Name: "queue.depth", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{
			{TimeUnixNano: uint64(time.Now().UnixNano()), Value: &metricspb.NumberDataPoint_AsInt{AsInt: 7}}}}}},
	}}}}}})

	post := func(path string, body []byte) int {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(body)
		_ = zw.Close()
		req := httptest.NewRequest(http.MethodPost, path, &buf)
		req.Header.Set("Content-Type", "application/x-protobuf")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if c := post("/api/v1/otlp/v1/logs", logBody); c != http.StatusNotImplemented {
		t.Fatalf("disabled = %d, want 501", c)
	}
	rt.runtimeSettings = runtimesettings.New(nil, func(k string) (string, bool) { return "true", k == "APP_OTLP_INGEST_ENABLED" })
	if c := post("/api/v1/otlp/v1/logs", logBody); c != http.StatusOK {
		t.Fatalf("logs = %d", c)
	}
	if c := post("/api/v1/otlp/v1/metrics", metBody); c != http.StatusOK {
		t.Fatalf("metrics = %d", c)
	}
	now := time.Now()
	lines, err := tel.QueryLogs(context.Background(), "service:web", now.Add(-time.Hour), now.Add(time.Hour), "")
	if err != nil || len(lines) != 1 || lines[0].Message != "disk full" || lines[0].Stream != "stderr" {
		t.Fatalf("logs = %+v err=%v", lines, err)
	}
	got, err := tel.QueryMany(context.Background(), []string{"service:web"}, []string{"otlp.queue.depth"}, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || len(got) != 1 || got[0].Value != 7 {
		t.Fatalf("samples = %+v err=%v", got, err)
	}
}

func decodeTracingKey(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var res tracingResource
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil || res.Key == "" {
		t.Fatalf("enable tracing = %d %s", rec.Code, rec.Body)
	}
	return res.Key
}
