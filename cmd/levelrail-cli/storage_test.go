package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func newStorageCLIServer(t *testing.T, probe apiclient.StorageProbeResult) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/storage/destinations":
			_ = json.NewEncoder(w).Encode([]apiclient.StorageDestination{{ID: "bkt_1", Name: "logs", Preset: "r2", Bucket: "b"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/storage/destinations":
			var req apiclient.StorageDestinationRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(apiclient.StorageDestination{ID: "bkt_2", Name: req.Name, Preset: req.Preset, Bucket: req.Bucket})
		case strings.HasSuffix(r.URL.Path, "/test"):
			_ = json.NewEncoder(w).Encode(probe)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/log-archive/policy":
			_ = json.NewEncoder(w).Encode(apiclient.LogArchivePolicy{ID: "lap_1", TargetID: "bkt_1", Interval: "1h0m0s", Enabled: true})
		case r.URL.Path == "/api/v1/log-archive/dump":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.LogArchiveRun{ID: "lar_1", Status: "succeeded", Lines: 3, Objects: 1})
		case r.URL.Path == "/api/v1/log-archive/objects":
			_ = json.NewEncoder(w).Encode(apiclient.LogArchiveObjects{Objects: []apiclient.LogArchiveObject{{Key: "log-archive/service/web/x.ndjson.gz", Size: 9}}})
		case r.URL.Path == "/api/v1/log-archive/objects/download":
			_, _ = w.Write([]byte("gzbytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestRun_StorageCommands(t *testing.T) {
	srv, calls := newStorageCLIServer(t, apiclient.StorageProbeResult{OK: true, Steps: []apiclient.StorageProbeStep{{Name: "put", OK: true}, {Name: "get", OK: true}, {Name: "delete", OK: true}}})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"list", []string{"storage", "list"}, "bkt_1"},
		{"add", []string{"storage", "add", "--name", "n", "--provider", "r2", "--account-id", "a", "--bucket", "b", "--access-key-id", "k", "--secret-access-key", "s"}, "bkt_2"},
		{"test", []string{"storage", "test", "bkt_1"}, "is working"},
		{"delete", []string{"storage", "delete", "bkt_1"}, "disconnected"},
		{"archive set", []string{"logs", "archive", "set", "--target", "bkt_1", "--interval", "1h"}, "policy for all apps set"},
		{"dump", []string{"logs", "dump", "--target", "bkt_1", "--from", "24h", "--wait"}, "3 lines in 1 objects"},
		{"ls", []string{"logs", "ls", "--target", "bkt_1", "--app", "web"}, "x.ndjson.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("stdout %q missing %q", stdout, tt.want)
			}
		})
	}
	if len(*calls) == 0 {
		t.Fatal("server saw no calls")
	}
}

func TestRun_StorageTestFailureExitsNonZero(t *testing.T) {
	srv, _ := newStorageCLIServer(t, apiclient.StorageProbeResult{Reason: "invalid_credentials", Message: "rejected", Steps: []apiclient.StorageProbeStep{{Name: "put", Error: "rejected"}}})
	var out, errb bytes.Buffer
	if got := run("levelrail-cli-test", []string{"storage", "test", "bkt_1", "--api-url", srv.URL}, &out, &errb, envMap()); got == exitOK {
		t.Fatalf("exit = %d, want failure", got)
	}
	if !strings.Contains(errb.String(), "invalid_credentials") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestRun_LogsFetchWritesFile(t *testing.T) {
	srv, _ := newStorageCLIServer(t, apiclient.StorageProbeResult{})
	dest := filepath.Join(t.TempDir(), "out.gz")
	runCLIExpectOK(t, []string{"logs", "fetch", "--target", "bkt_1", "--key", "log-archive/x.ndjson.gz", "--out", dest, "--api-url", srv.URL})
	data, err := os.ReadFile(dest) //nolint:gosec // test temp path
	if err != nil || string(data) != "gzbytes" {
		t.Fatalf("file = %q, %v", data, err)
	}
}

func TestRun_LogsValidation(t *testing.T) {
	var out, errb bytes.Buffer
	for _, args := range [][]string{
		{"logs", "dump", "--target", "bkt_1", "--api-url", "http://127.0.0.1:1"},
		{"logs", "dump", "--target", "bkt_1", "--from", "garbage", "--api-url", "http://127.0.0.1:1"},
		{"storage", "add", "--name", "n", "--api-url", "http://127.0.0.1:1"},
		{"logs", "archive", "set", "--api-url", "http://127.0.0.1:1"},
	} {
		if got := run("levelrail-cli-test", args, &out, &errb, envMap()); got != exitValidation {
			t.Fatalf("%v exit = %d, want %d", args, got, exitValidation)
		}
	}
}
