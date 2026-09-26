package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsDeploysCancel(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deployAttemptResource{ID: "dep_2", ServiceName: "web", Status: "canceled", CanceledBy: "alice"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "cancel", "web", "dep_2", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/deploys/dep_2/cancel" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "dep_2") || !strings.Contains(stdout, "canceled") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRun_AppsDeploysCancel_ConflictIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"this deploy already cut over to the new release"}`)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploys", "cancel", "web", "dep_2", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK || !strings.Contains(stderr.String(), "cut over") {
		t.Fatalf("exit = %d, stderr = %q", got, stderr.String())
	}
}

func TestRun_AppsDeploysCancel_NeedsTwoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"apps", "deploys", "cancel", "web"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsDeploysRollbackTo(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"name":"web","image":"nginx:1@sha256:aaa","port":80}`)
	}))
	defer srv.Close()

	_, stderr := runCLIExpectOK(t, []string{"apps", "deploys", "rollback-to", "web", "dep_old", "--override-freeze", "--override-reason", "hotfix", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/deploys/dep_old/rollback" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["override_freeze"] != true || gotBody["override_reason"] != "hotfix" {
		t.Fatalf("body = %v", gotBody)
	}
	if !strings.Contains(stderr, "nginx:1@sha256:aaa") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRun_AppsCancelSuperseded(t *testing.T) {
	var last string
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.Method + " " + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"enabled":true}`)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "cancel-superseded", "enable", "web", "--api-url", srv.URL})
	if last != "PUT /api/v1/apps/web/cancel-superseded" || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("request = %s %s", last, body)
	}
	if !strings.Contains(stdout, "cancel-superseded: true") {
		t.Fatalf("stdout = %q", stdout)
	}
	runCLIExpectOK(t, []string{"apps", "cancel-superseded", "status", "web", "--api-url", srv.URL})
	if last != "GET /api/v1/apps/web/cancel-superseded" {
		t.Fatalf("status request = %s", last)
	}
	runCLIExpectOK(t, []string{"apps", "cancel-superseded", "disable", "web", "--api-url", srv.URL})
	if !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("disable body = %s", body)
	}
}
