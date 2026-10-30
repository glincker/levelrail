package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func waitServer(t *testing.T, outcomes []string, paths *[]string) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		i := min(int(calls.Add(1))-1, len(outcomes)-1)
		d := deployAttemptResource{ID: "dep_9", ServiceName: "web", Status: "running", Outcome: outcomes[i]}
		if outcomes[i] == "failed" {
			d.Status = "failed"
			d.Failure = &apiclient.DeployFailure{Code: "health_check_failed", Cause: "never ready", SuggestedFix: "raise timeout", DocsURL: "/deploy-failures#health_check_failed"}
		}
		_ = json.NewEncoder(w).Encode(d)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsDeploysWait(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []string
		extra    []string
		wantExit int
		wantOut  string
	}{
		{"healthy", []string{"in_progress", "healthy"}, nil, exitOK, "healthy"},
		{"failed exits 7 with the failure", []string{"failed"}, nil, exitDeployNotHealthy, "health_check_failed"},
		{"canceled exits 7", []string{"canceled"}, nil, exitDeployNotHealthy, "canceled"},
		{"timeout exits 6", []string{"in_progress"}, []string{"--timeout", "30ms"}, exitDeployTimeout, "timed out"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			srv := waitServer(t, tc.outcomes, &paths)
			args := append([]string{"apps", "deploys", "wait", "web", "--poll-interval", "5ms", "--api-url", srv.URL}, tc.extra...)
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr %q)", got, tc.wantExit, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout %q missing %q", stdout.String(), tc.wantOut)
			}
			if len(paths) > 1 && paths[1] != "/api/v1/apps/web/deploys/dep_9" {
				t.Errorf("latest not pinned to the concrete id: %v", paths)
			}
		})
	}
}

func TestRun_AppsDeploysWait_FrozenAfterRefusedDeploy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deploy-freeze") {
			_, _ = w.Write([]byte(`{"windows":[],"status":{"frozen":true,"reason":"release freeze"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(deployAttemptResource{ID: "dep_old", ServiceName: "web", Status: "succeeded", Outcome: "healthy"})
	}))
	t.Cleanup(srv.Close)
	for _, tc := range []struct {
		name     string
		extra    []string
		wantExit int
	}{
		{"implicit latest is blocked", nil, exitDeployNotHealthy},
		{"explicit id still reports healthy", []string{"dep_old"}, exitOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"apps", "deploys", "wait", "web"}, tc.extra...)
			args = append(args, "--poll-interval", "5ms", "--api-url", srv.URL)
			var stdout, stderr bytes.Buffer
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stdout %q stderr %q)", got, tc.wantExit, stdout.String(), stderr.String())
			}
			if tc.wantExit != exitOK && !strings.Contains(stderr.String(), "frozen") {
				t.Errorf("stderr %q should name the freeze", stderr.String())
			}
		})
	}
}

func TestRun_AppsDeploysWait_CrashLoopLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/apps/web/deploys" {
			_, _ = w.Write([]byte(`[{"type":"Ready","status":"False","reason":"CrashLoopBackOff","message":"\"web-1\" keeps exiting (3 restarts), next restart in 20s","last_transition_time":"2026-10-24T10:00:00Z"}]`))
			return
		}
		_ = json.NewEncoder(w).Encode(deployAttemptResource{ID: "dep_9", ServiceName: "web", Status: "failed", Outcome: "failed",
			Failure: &apiclient.DeployFailure{Code: "container_crashed", Cause: "exited"}})
	}))
	t.Cleanup(srv.Close)
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "wait", "web", "dep_9", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitDeployNotHealthy {
		t.Fatalf("exit = %d, want %d (stderr %q)", got, exitDeployNotHealthy, stderr.String())
	}
	if want := "crash loop detected: the release restarted 3 times, next retry in 20s"; !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout %q missing %q", stdout.String(), want)
	}
}

func TestCrashLoopLine(t *testing.T) {
	if got := crashLoopLine(nil, false); got != "" {
		t.Errorf("healthy app got %q", got)
	}
	if got := crashLoopLine(nil, true); !strings.Contains(got, "container crash") {
		t.Errorf("crash without backoff got %q", got)
	}
}
