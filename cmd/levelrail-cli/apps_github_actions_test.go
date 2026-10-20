package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsGitHubActionsSetAndGet(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"environment": "staging", "report_deployments": true, "report_checks": false})
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"apps", "github-actions", "set", "web", "--environment", "staging", "--no-checks", "--api-url", srv.URL})
	if gotBody["environment"] != "staging" || gotBody["report_checks"] != false || gotBody["report_deployments"] != true {
		t.Errorf("body = %v", gotBody)
	}
	if !strings.Contains(out, "staging") {
		t.Errorf("stdout = %q", out)
	}
	out, _ = runCLIExpectOK(t, []string{"apps", "github-actions", "get", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "checks false") {
		t.Errorf("stdout = %q", out)
	}
}

// Check-run calls fail but the deploy-side outputs and exit code stay clean.
func TestRun_AppsGitHubActionsStartFinishHalfSucceeded(t *testing.T) {
	t.Setenv("APP_GIT_ALLOW_PRIVATE_NETWORKS", "true")
	var hits []string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.Path)
		if strings.Contains(r.URL.Path, "check-runs") {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"id": 7}`))
	}))
	defer gh.Close()
	lr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"environment": "production", "report_deployments": true, "report_checks": true})
	}))
	defer lr.Close()

	env := map[string]string{"GITHUB_TOKEN": "t", "GITHUB_REPOSITORY": "o/r", "GITHUB_SHA": "abc", "GITHUB_SERVER_URL": gh.URL}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"apps", "github-actions", "start", "web", "--api-url", lr.URL}, &stdout, &stderr, lookup); code != exitOK {
		t.Fatalf("start exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deployment_id=7") || !strings.Contains(stdout.String(), "check_run_id=\n") {
		t.Errorf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning: GitHub report failed") {
		t.Errorf("stderr = %q, want a warning", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	args := []string{"apps", "github-actions", "finish", "web", "--outcome", "succeeded", "--deployment-id", "7", "--api-url", lr.URL}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, lookup); code != exitOK {
		t.Fatalf("finish exit = %d, stderr = %s", code, stderr.String())
	}
	if last := hits[len(hits)-1]; !strings.HasSuffix(last, "/deployments/7/statuses") {
		t.Errorf("last GitHub call = %q", last)
	}
}

func TestRun_AppsGitHubActionsStartNeedsToken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"apps", "github-actions", "start", "web", "--api-url", "http://127.0.0.1:1"}, &stdout, &stderr, envMap()); code == exitOK {
		t.Fatal("want a validation failure without GITHUB_TOKEN")
	}
}
