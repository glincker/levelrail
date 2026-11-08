package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestAppsRunStreamsOutputAndPassesTimeout(t *testing.T) {
	var gotReq apiclient.OneShotJobRequest
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/runs":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotReq)
			_ = json.NewEncoder(w).Encode(apiclient.OneShotJob{ID: "jb_s", State: "Running"})
		case r.URL.Path == "/api/v1/jobs/runs/jb_s":
			polls++
			j := apiclient.OneShotJob{ID: "jb_s", State: "Running", Output: "a\n"}
			if polls > 1 {
				j.State, j.Output = "Succeeded", "a\nb\n"
			}
			_ = json.NewEncoder(w).Encode(j)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	args := []string{"apps", "run", "web", "--api-url", srv.URL, "--timeout", "30s", "--poll", "1ms", "--", "seq", "2"}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	if stdout.String() != "a\nb\n" {
		t.Fatalf("stdout = %q, want each line once", stdout.String())
	}
	if gotReq.TimeoutSeconds != 30 {
		t.Fatalf("timeout_seconds = %d, want 30", gotReq.TimeoutSeconds)
	}
}
