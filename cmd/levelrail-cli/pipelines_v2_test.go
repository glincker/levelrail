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

func TestRun_PipelinesRunParam(t *testing.T) {
	var body apiclient.PipelineStartRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.PipelineRunResource{ID: "r1", Number: 2})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	args := []string{"pipelines", "run", "web", "ci", "--api-url", srv.URL, "--param", "env=prod", "--param", "dry_run=true", "--input", "note=x"}
	if got := run("cli", args, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit %d stderr %q", got, stderr.String())
	}
	if body.Inputs["env"] != "prod" || body.Inputs["dry_run"] != "true" || body.Inputs["note"] != "x" {
		t.Errorf("inputs = %v", body.Inputs)
	}
}

func TestRun_PipelinesArtifactsListAndDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/apps/web/pipeline-runs/r1/artifacts":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]apiclient.PipelineArtifact{{ID: "a1", Job: "build", SizeBytes: 5, SHA256: "abc"}})
		case "/api/v1/apps/web/pipeline-runs/r1/artifacts/a1/download":
			_, _ = w.Write([]byte("bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	if got := run("cli", []string{"pipelines", "artifacts", "web", "r1", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK || !strings.Contains(stdout.String(), "build") {
		t.Fatalf("list: exit %d out %q err %q", got, stdout.String(), stderr.String())
	}
	out := filepath.Join(t.TempDir(), "a.tar.gz")
	stdout.Reset()
	if got := run("cli", []string{"pipelines", "artifacts", "web", "r1", "--api-url", srv.URL, "--download", "build", "--out", out}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("download: exit %d err %q", got, stderr.String())
	}
	if b, _ := os.ReadFile(out); string(b) != "bytes" { //nolint:gosec // out is a t.TempDir path
		t.Errorf("downloaded %q", b)
	}
	stdout.Reset()
	stderr.Reset()
	if got := run("cli", []string{"pipelines", "artifacts", "web", "r1", "--api-url", srv.URL, "--download", "nope"}, &stdout, &stderr, envMap()); got == exitOK {
		t.Error("downloading an unknown job's artifact succeeded")
	}
}

func TestRun_PipelinesTriggersWebhook(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/deliveries"):
			_ = json.NewEncoder(w).Encode([]apiclient.PipelineWebhookDelivery{{ID: "d1", Outcome: "started", StatusCode: 202, RunID: "r9"}})
		case strings.HasSuffix(r.URL.Path, "/replay"):
			_ = json.NewEncoder(w).Encode(apiclient.PipelineRunResource{ID: "r10", Number: 10})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_ = json.NewEncoder(w).Encode(apiclient.PipelineWebhook{Configured: true, Enabled: true, HMAC: true, Path: "/api/v1/hooks/pipeline/p1/tok", Token: "tok", Secret: "sec"}) //nolint:gosec // fake secret in a test fixture
		}
	}))
	defer srv.Close()
	cases := []struct {
		args []string
		want string
		call string
	}{
		{[]string{"--enable", "--hmac"}, "Signing secret (shown once): sec", "PUT /api/v1/apps/web/pipelines/ci/webhook"},
		{[]string{}, "POST /api/v1/hooks/pipeline/p1/tok", "GET /api/v1/apps/web/pipelines/ci/webhook"},
		{[]string{"--rotate"}, "shown once", "POST /api/v1/apps/web/pipelines/ci/webhook/rotate"},
		{[]string{"--deliveries"}, "d1", "GET /api/v1/apps/web/pipelines/ci/webhook/deliveries"},
		{[]string{"--replay", "d1"}, "run #10 started", "POST /api/v1/apps/web/pipelines/ci/webhook/deliveries/d1/replay"},
		{[]string{"--delete"}, "deleted", "DELETE /api/v1/apps/web/pipelines/ci/webhook"},
	}
	for _, tc := range cases {
		seen = nil
		var stdout, stderr bytes.Buffer
		args := append([]string{"pipelines", "triggers", "web", "ci", "--api-url", srv.URL}, tc.args...)
		if got := run("cli", args, &stdout, &stderr, envMap()); got != exitOK || !strings.Contains(stdout.String(), tc.want) {
			t.Errorf("%v: exit %d out %q err %q", tc.args, got, stdout.String(), stderr.String())
		}
		if len(seen) != 1 || seen[0] != tc.call {
			t.Errorf("%v: calls = %v, want %q", tc.args, seen, tc.call)
		}
	}
}
