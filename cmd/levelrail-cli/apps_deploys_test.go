package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_AppsDeploysList(t *testing.T) {
	var gotPath string
	finishedAt, err := time.Parse(time.RFC3339, "2026-01-01T00:00:05Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]deployAttemptResource{
			{ID: "dep_2", ServiceName: "web", Image: "levelrail/web:2", Source: "image", Status: "succeeded", StartedAt: finishedAt, FinishedAt: &finishedAt},
			{ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1", Source: "image", Status: "failed", StartedAt: finishedAt, Error: "build failed"},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "list", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/deploy-attempts" {
		t.Errorf("path = %q, want the deploy-attempts endpoint", gotPath)
	}
	if !strings.Contains(stdout, "dep_2") || !strings.Contains(stdout, "dep_1") {
		t.Errorf("stdout = %q, want both attempt ids listed", stdout)
	}
	if !strings.Contains(stdout, "build failed") {
		t.Errorf("stdout = %q, want the failed attempt's error shown", stdout)
	}
}

func TestRun_AppsDeploysList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]deployAttemptResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no deploy attempts recorded yet") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

func TestRun_AppsDeploysList_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "list"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsDeploysCompare(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(deployCompareResource{
			ServiceName: "web",
			From:        deployCompareSide{DeployID: "dep_1", Image: "levelrail/web:1", Status: "succeeded"},
			To:          deployCompareSide{DeployID: "dep_2", Image: "levelrail/web:2", Status: "succeeded"},
			Changes: []deployCompareField{
				{Field: "image", From: "levelrail/web:1", To: "levelrail/web:2"},
			},
			UnsnapshottedFields: []string{"env", "resources"},
			Note:                "not tracked per attempt",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "compare", "web", "--from", "dep_1", "--to", "dep_2", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/apps/web/deploys/compare?from=dep_1&to=dep_2" {
		t.Errorf("path = %q, want from/to query params", gotPath)
	}
	if !strings.Contains(stdout.String(), `"image"`) {
		t.Errorf("stdout = %q, want the comparison as JSON", stdout.String())
	}
}

func TestRun_AppsDeploysCompare_AgainstCurrent(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(deployCompareResource{
			ServiceName:         "web",
			From:                deployCompareSide{DeployID: "dep_1", Image: "levelrail/web:1", Status: "succeeded"},
			To:                  deployCompareSide{IsCurrent: true, Image: "levelrail/web:current"},
			UnsnapshottedFields: []string{"env"},
			Note:                "not tracked per attempt",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "compare", "web", "--from", "dep_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/deploys/compare?from=dep_1" {
		t.Errorf("path = %q, want only from set when --to is omitted", gotPath)
	}
	if !strings.Contains(stdout, "current live state") {
		t.Errorf("stdout = %q, want the current-state label", stdout)
	}
	if !strings.Contains(stdout, "not tracked per deploy attempt") {
		t.Errorf("stdout = %q, want the honest limitation note", stdout)
	}
}

func TestRun_AppsDeploysCompare_MissingFrom(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "compare", "web"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--from is required") {
		t.Errorf("stderr = %q, want a missing --from validation error", stderr.String())
	}
}

func TestRun_AppsDeploysCompare_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "compare", "--from", "dep_1"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsDeploysCompare_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"deploy attempt not found"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "compare", "web", "--from", "dep_ghost", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "deploy attempt not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsDeploysCompare_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "compare", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps deploys compare") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func TestRun_AppsDeploys_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps deploys compare") {
		t.Errorf("stdout = %q, want the deploys subcommand list", stdout.String())
	}
}

func TestRun_AppsDeploys_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps deploys subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AppsDeploysLogs(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("stdout line one\nstderr line two\n"))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "logs", "web", "dep_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/deploys/dep_1/logs/download" {
		t.Errorf("path = %q, want the deploy log download endpoint", gotPath)
	}
	if !strings.Contains(stdout, "stdout line one") || !strings.Contains(stdout, "stderr line two") {
		t.Errorf("stdout = %q, want the raw log body written through unmodified", stdout)
	}
}

func TestRun_AppsDeploysLogs_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "logs", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
}

func TestRun_AppsDeploysLogs_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"deploy attempt not found"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "logs", "web", "dep_ghost", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "deploy attempt not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsDeploysLogs_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "logs", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps deploys logs") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
