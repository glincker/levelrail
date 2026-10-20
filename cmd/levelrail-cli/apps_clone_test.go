package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsClone(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody cloneAppRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(appResource{Name: gotBody.NewName, Image: "nginx:1", Port: 80})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "clone", "web", "web-copy", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/clone" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/clone", gotMethod, gotPath)
	}
	if gotBody.NewName != "web-copy" {
		t.Errorf("request body NewName = %q, want web-copy", gotBody.NewName)
	}
	if !strings.Contains(stdout, "web-copy") {
		t.Errorf("stdout = %q, want the cloned app name shown", stdout)
	}
}

func TestRun_AppsClone_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "clone", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires an app name and a new name") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsClone_Conflict(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"an app with this name already exists"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "clone", "web", "web", "--api-url", srv.URL})
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("stderr = %q, want the server's conflict message", stderr)
	}
}

func TestRun_AppsClone_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "clone", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps clone") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
