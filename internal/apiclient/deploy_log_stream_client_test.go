package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_StreamDeployLog proves StreamDeployLog hits the deploy-log
// SSE route (not StreamLogs' /logs/stream route, a different endpoint on
// the same wire shape) and decodes entries the same way StreamLogs does.
func TestClient_StreamDeployLog(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test ResponseWriter does not implement http.Flusher")
		}
		_, _ = fmt.Fprint(w, ": connected\n\n"+
			`data: {"line":"Cloning https://example.invalid/repo.git...","stream":"stdout"}`+"\n\n"+
			`data: {"line":"Checked out sha1","stream":"stdout"}`+"\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-token")
	var got []LogStreamEntry
	err := client.StreamDeployLog(context.Background(), "web", "dep_1", func(e LogStreamEntry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamDeployLog() error = %v", err)
	}
	if gotPath != "/api/v1/apps/web/deploys/dep_1/logs" {
		t.Errorf("path = %q, want /api/v1/apps/web/deploys/dep_1/logs", gotPath)
	}
	if len(got) != 2 {
		t.Fatalf("StreamDeployLog() delivered %d entries, want 2 (got %+v)", len(got), got)
	}
	if got[0].Line != "Cloning https://example.invalid/repo.git..." {
		t.Errorf("entry[0].Line = %q, want the clone-start line", got[0].Line)
	}
	if got[1].Line != "Checked out sha1" {
		t.Errorf("entry[1].Line = %q, want the checkout-complete line", got[1].Line)
	}
}

// TestClient_StreamDeployLog_CallbackStopsEarly mirrors
// TestClient_StreamLogs_CallbackStopsEarly: a non-nil onEntry return must
// stop the stream and be returned as-is, since streamLogEvents is shared
// between both methods.
func TestClient_StreamDeployLog_CallbackStopsEarly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test ResponseWriter does not implement http.Flusher")
		}
		_, _ = fmt.Fprint(w, `data: {"line":"one","stream":"stdout"}`+"\n\n"+
			`data: {"line":"two","stream":"stdout"}`+"\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)

	stopErr := errors.New("stop")
	client := NewClient(srv.URL, "test-token")
	var got []LogStreamEntry
	err := client.StreamDeployLog(context.Background(), "web", "dep_1", func(e LogStreamEntry) error {
		got = append(got, e)
		return stopErr
	})
	if !errors.Is(err, stopErr) {
		t.Fatalf("StreamDeployLog() error = %v, want stopErr", err)
	}
	if len(got) != 1 {
		t.Errorf("StreamDeployLog() delivered %d entries before stopping, want 1", len(got))
	}
}
