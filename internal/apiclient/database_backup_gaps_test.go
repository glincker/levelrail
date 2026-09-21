package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_QueryDatabaseLogs(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entries":[{"message":"db ready","stream":"stdout"}]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	got, err := client.QueryDatabaseLogs(context.Background(), "main", from, to, "ready")
	if err != nil {
		t.Fatalf("QueryDatabaseLogs() error = %v", err)
	}
	if len(got) != 1 || got[0].Message != "db ready" {
		t.Errorf("QueryDatabaseLogs() = %+v, want one entry \"db ready\"", got)
	}
	if want := "/api/v1/databases/main/logs?from=2026-01-01T00%3A00%3A00Z&q=ready&to=2026-01-02T00%3A00%3A00Z"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
}

func TestClient_SetDatabaseNode(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetAppNodeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"main","node_id":"node-2"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetDatabaseNode(context.Background(), "main", "node-2")
	if err != nil {
		t.Fatalf("SetDatabaseNode() error = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/databases/main/node" {
		t.Errorf("path = %q, want /api/v1/databases/main/node", gotPath)
	}
	if gotBody.NodeID != "node-2" {
		t.Errorf("request body node_id = %q, want node-2", gotBody.NodeID)
	}
	if got.NodeID != "node-2" {
		t.Errorf("SetDatabaseNode() = %+v, want NodeID node-2", got)
	}
}

func TestClient_DownloadBackup(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("dump-bytes"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DownloadBackup(context.Background(), "main", "hist-1")
	if err != nil {
		t.Fatalf("DownloadBackup() error = %v", err)
	}
	if string(got) != "dump-bytes" {
		t.Errorf("DownloadBackup() = %q, want %q", got, "dump-bytes")
	}
	if want := "/api/v1/databases/main/backups/hist-1/download"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestClient_DownloadVolumeBackup(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("volume-dump-bytes"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DownloadVolumeBackup(context.Background(), "web", "data", "hist-2")
	if err != nil {
		t.Fatalf("DownloadVolumeBackup() error = %v", err)
	}
	if string(got) != "volume-dump-bytes" {
		t.Errorf("DownloadVolumeBackup() = %q, want %q", got, "volume-dump-bytes")
	}
	if want := "/api/v1/apps/web/volumes/data/backups/hist-2/download"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestClient_DownloadBackup_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"backup history not found"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if _, err := client.DownloadBackup(context.Background(), "main", "missing"); err == nil {
		t.Fatal("DownloadBackup() error = nil, want an error for a 404 response")
	}
}

func TestClient_StreamDatabaseLogs(t *testing.T) {
	srv := sseServer(t, `data: {"line":"checkpoint complete","stream":"stdout"}`+"\n\n")

	client := NewClient(srv.URL, "test-token")
	var got []LogStreamEntry
	err := client.StreamDatabaseLogs(context.Background(), "main", func(e LogStreamEntry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamDatabaseLogs() error = %v", err)
	}
	if len(got) != 1 || got[0].Line != "checkpoint complete" {
		t.Errorf("StreamDatabaseLogs() = %+v, want one entry \"checkpoint complete\"", got)
	}
}
