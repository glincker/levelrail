package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAppsUsage_BuildsTriggerFlagsExist guards the apps summary line
// against naming a flag the subcommand does not accept.
func TestAppsUsage_BuildsTriggerFlagsExist(t *testing.T) {
	var line string
	for _, l := range strings.Split(appsUsage("cli"), "\n") {
		if strings.Contains(l, "apps builds trigger") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("apps usage has no builds trigger line")
	}
	var stdout, stderr bytes.Buffer
	run("cli", []string{"apps", "builds", "trigger", "-h"}, &stdout, &stderr, envMap())
	help := stdout.String() + stderr.String()
	for _, tok := range strings.Fields(line) {
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		if !strings.Contains(help, "-"+strings.TrimPrefix(tok, "--")+" ") {
			t.Errorf("summary names %s, which apps builds trigger -h does not list", tok)
		}
	}
}

func TestRun_AppsBuilds_Trigger(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody buildTriggerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(buildTriggerResponse{ID: "deploy_1"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"apps", "builds", "trigger", "web",
		"--repo", "https://example.com/org/web.git", "--ref", "main", "--image-repo", "registry.example.com/org/web",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/builds" {
		t.Errorf("method/path = %s %s, want POST /api/v1/apps/web/builds", gotMethod, gotPath)
	}
	if gotBody.RepoURL != "https://example.com/org/web.git" || gotBody.Ref != "main" {
		t.Errorf("request body = %+v, want repo_url/ref set", gotBody)
	}
	if !strings.Contains(stdout, "deploy_1") {
		t.Errorf("stdout = %q, want the deploy attempt id", stdout)
	}
}

func TestRun_AppsBuilds_Trigger_BuildArgs(t *testing.T) {
	var gotBody buildTriggerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(buildTriggerResponse{ID: "deploy_1"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"apps", "builds", "trigger", "web",
		"--repo", "https://example.com/org/web.git", "--ref", "main",
		"--build-arg", "FOO=bar", "--api-url", srv.URL,
	})

	if gotBody.Build.Args["FOO"] != "bar" {
		t.Errorf("request body build.args = %+v, want FOO=bar", gotBody.Build.Args)
	}
}

// TestRun_AppsBuilds_Trigger_DetectsFramework covers F-018's CLI side
// for a manual rebuild: "apps builds trigger" runs the same pre-flight
// framework detection the web wizard runs and forwards the result as
// detected_framework, so a rebuild triggered from the CLI records a
// FRAMEWORK value too.
func TestRun_AppsBuilds_Trigger_DetectsFramework(t *testing.T) {
	var gotBody buildTriggerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/build/detect":
			_, _ = w.Write([]byte(`{"provider":"node","framework_name":"Next.js","detected":true}`))
		default:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(buildTriggerResponse{ID: "deploy_1"})
		}
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"apps", "builds", "trigger", "web",
		"--repo", "https://example.com/org/web.git", "--ref", "main",
		"--api-url", srv.URL,
	})

	if gotBody.DetectedFramework != "Next.js" {
		t.Errorf("request body detected_framework = %q, want %q", gotBody.DetectedFramework, "Next.js")
	}
}

func TestRun_AppsBuilds_Trigger_MissingRepo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "builds", "trigger", "web", "--ref", "main", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --repo") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsBuilds_Trigger_MissingRef(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "builds", "trigger", "web", "--repo", "https://example.com/x.git", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --ref") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsBuilds_Trigger_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"resolve ref failed"}`)

	stderr := runCLIExpectAPIError(t, []string{
		"apps", "builds", "trigger", "web",
		"--repo", "https://example.com/org/web.git", "--ref", "bogus", "--api-url", srv.URL,
	})
	if !strings.Contains(stderr, "resolve ref failed") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_AppsBuilds_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "builds", "-h"})
	if !strings.Contains(stdout, "apps builds trigger") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsBuilds_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "builds", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps builds subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
