package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_QueryAppMetrics(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AppMetricsResource{
			Metric: "cpu_percent",
			Points: []MetricPointResource{{Timestamp: time.Unix(0, 0).UTC(), Value: 12.5, Count: 3}},
		})
	}))
	defer srv.Close()

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	client := NewClient(srv.URL, "test-token")
	got, err := client.QueryAppMetrics(context.Background(), "web", "cpu_percent", from, to, 30*time.Second)
	if err != nil {
		t.Fatalf("QueryAppMetrics() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/metrics" {
		t.Errorf("request = %s %s, want GET /api/v1/apps/web/metrics", gotMethod, gotPath)
	}
	if !strings.Contains(gotQuery, "metric=cpu_percent") || !strings.Contains(gotQuery, "step=30s") {
		t.Errorf("query = %q, want metric and step params", gotQuery)
	}
	if got.Metric != "cpu_percent" || len(got.Points) != 1 || got.Points[0].Value != 12.5 {
		t.Errorf("QueryAppMetrics() = %+v, want one 12.5 point", got)
	}
}

func TestClient_QueryAppMetrics_ZeroStepOmitted(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AppMetricsResource{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.QueryAppMetrics(context.Background(), "web", "cpu_percent", time.Now(), time.Now(), 0)
	if err != nil {
		t.Fatalf("QueryAppMetrics() error = %v", err)
	}
	if strings.Contains(gotQuery, "step=") {
		t.Errorf("query = %q, want no step param when step is zero", gotQuery)
	}
}

func TestClient_QueryDatabaseMetrics(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AppMetricsResource{
			Metric: "memory_usage_bytes",
			Points: []MetricPointResource{{Value: 1024, Count: 1}},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.QueryDatabaseMetrics(context.Background(), "main", "memory_usage_bytes", time.Now().Add(-time.Hour), time.Now(), 0)
	if err != nil {
		t.Fatalf("QueryDatabaseMetrics() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/databases/main/metrics" {
		t.Errorf("request = %s %s, want GET /api/v1/databases/main/metrics", gotMethod, gotPath)
	}
	if got.Metric != "memory_usage_bytes" || len(got.Points) != 1 || got.Points[0].Value != 1024 {
		t.Errorf("QueryDatabaseMetrics() = %+v, want one 1024 point", got)
	}
}

func TestClient_QueryDatabaseMetrics_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"database not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.QueryDatabaseMetrics(context.Background(), "missing", "cpu_percent", time.Now(), time.Now(), 0)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("QueryDatabaseMetrics() error = %v, want a 404 *APIError", err)
	}
}

func TestClient_QueryNodeMetrics(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NodeMetricsResource{
			Metric:        "cpu_percent",
			Points:        []MetricPointResource{{Value: 42, Count: 2}},
			ResourceCount: 2,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.QueryNodeMetrics(context.Background(), "nd_1", "cpu_percent", time.Now().Add(-time.Hour), time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("QueryNodeMetrics() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/nodes/nd_1/metrics" {
		t.Errorf("request = %s %s, want GET /api/v1/nodes/nd_1/metrics", gotMethod, gotPath)
	}
	if got.ResourceCount != 2 || len(got.Points) != 1 || got.Points[0].Value != 42 {
		t.Errorf("QueryNodeMetrics() = %+v, want ResourceCount=2 and one 42 point", got)
	}
}

func TestClient_QueryNodeMetrics_BadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"metric must be one of cpu_percent, memory_usage_bytes"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.QueryNodeMetrics(context.Background(), "nd_1", "memory_limit_bytes", time.Now(), time.Now(), 0)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("QueryNodeMetrics() error = %v, want a 400 *APIError", err)
	}
}

// sseServer starts an httptest.Server that writes rawBody (already in SSE
// "data: ...\n\n" wire format) as a flushed streaming response, then
// closes the connection: StreamLogs must treat that close as a clean
// end-of-stream, not an error, the same way handleLiveLogStream's real
// server holds the connection open only until its own context ends.
func sseServer(t *testing.T, rawBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test ResponseWriter does not implement http.Flusher")
		}
		_, _ = fmt.Fprint(w, rawBody)
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_StreamLogs(t *testing.T) {
	srv := sseServer(t, ": connected\n\n"+
		`data: {"line":"starting up","stream":"stdout"}`+"\n\n"+
		`data: {"line":"listening on :3000","stream":"stdout"}`+"\n\n")

	client := NewClient(srv.URL, "test-token")
	var got []LogStreamEntry
	err := client.StreamLogs(context.Background(), "web", func(e LogStreamEntry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("StreamLogs() delivered %d entries, want 2 (got %+v)", len(got), got)
	}
	if got[0].Line != "starting up" || got[0].Stream != "stdout" {
		t.Errorf("entry[0] = %+v, want {starting up stdout}", got[0])
	}
	if got[1].Line != "listening on :3000" {
		t.Errorf("entry[1] = %+v, want line \"listening on :3000\"", got[1])
	}
}

func TestClient_StreamLogs_CallbackStopsEarly(t *testing.T) {
	srv := sseServer(t,
		`data: {"line":"one","stream":"stdout"}`+"\n\n"+
			`data: {"line":"two","stream":"stdout"}`+"\n\n")

	client := NewClient(srv.URL, "test-token")
	stopErr := errors.New("stop here")
	var got []string
	err := client.StreamLogs(context.Background(), "web", func(e LogStreamEntry) error {
		got = append(got, e.Line)
		return stopErr
	})
	if !errors.Is(err, stopErr) {
		t.Fatalf("StreamLogs() error = %v, want the callback's own stopErr returned unwrapped", err)
	}
	if len(got) != 1 || got[0] != "one" {
		t.Errorf("delivered entries = %v, want exactly [\"one\"] before the callback stopped it", got)
	}
}

func TestClient_StreamLogs_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	err := client.StreamLogs(context.Background(), "missing", func(LogStreamEntry) error { return nil })
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("StreamLogs() error = %v, want a 404 *APIError", err)
	}
}

func TestClient_StreamLogs_ContextCanceled(t *testing.T) {
	srv := sseServer(t, `data: {"line":"one","stream":"stdout"}`+"\n\n")

	client := NewClient(srv.URL, "test-token")
	ctx, cancel := context.WithCancel(context.Background())
	err := client.StreamLogs(ctx, "web", func(LogStreamEntry) error {
		cancel()
		return nil
	})
	// The test server closes its body right after writing one event, so
	// this mostly exercises that a subsequently-canceled context doesn't
	// turn a clean server-side close into a reported error.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("StreamLogs() error = %v, want nil or context.Canceled", err)
	}
}
