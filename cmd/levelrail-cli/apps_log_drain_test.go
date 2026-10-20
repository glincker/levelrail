package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsLogDrainSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setLogDrainRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(logDrainResource{AppName: "web", Type: "http", Target: "https://collector.example.com", Enabled: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "set", "web", "--type", "http", "--target", "https://collector.example.com", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/log-drain" {
		t.Errorf("path = %q, want /api/v1/apps/web/log-drain", gotPath)
	}
	if gotBody.Type != "http" || gotBody.Target != "https://collector.example.com" || !gotBody.Enabled {
		t.Errorf("request body = %+v, want type=http target=https://collector.example.com enabled=true", gotBody)
	}
	if !strings.Contains(stdout.String(), `"type": "http"`) {
		t.Errorf("stdout = %q, want the drain as JSON", stdout.String())
	}
}

func TestRun_AppsLogDrainSet_Disabled(t *testing.T) {
	var gotBody setLogDrainRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(logDrainResource{AppName: "web", Type: "syslog", Target: "udp://logs.example.com:514", Enabled: false})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "set", "web", "--type", "syslog", "--target", "udp://logs.example.com:514", "--disabled", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false with --disabled")
	}
	if !strings.Contains(stdout.String(), "status: disabled") {
		t.Errorf("stdout = %q, want the human summary to show disabled", stdout.String())
	}
}

func TestRun_AppsLogDrainSet_MissingType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "set", "web", "--target", "https://collector.example.com"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires --type") {
		t.Errorf("stderr = %q, want a missing --type usage error", stderr.String())
	}
}

func TestRun_AppsLogDrainGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/log-drain" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/log-drain", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(logDrainResource{AppName: "web", Type: "http", Target: "https://collector.example.com", Enabled: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "get", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "target: https://collector.example.com") {
		t.Errorf("stdout = %q, want the target line", stdout.String())
	}
}

func TestRun_AppsLogDrainGet_NotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no log drain configured for this app"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "get", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitAPIError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no log drain configured") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsLogDrainClear(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "clear", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/log-drain" {
		t.Errorf("path = %q, want /api/v1/apps/web/log-drain", gotPath)
	}
	if !strings.Contains(stdout.String(), `log drain removed for app "web"`) {
		t.Errorf("stdout = %q, want a removal confirmation", stdout.String())
	}
}

func TestRun_AppsLogDrainClear_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "clear", "web", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var body map[string]bool
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if !body["cleared"] {
		t.Errorf("stdout = %q, want {\"cleared\": true}", stdout.String())
	}
}

func TestRun_AppsLogDrain_NoName(t *testing.T) {
	assertUsageErrorMissingName(t, "log-drain", []string{"get", "set", "clear"})
}

func TestRun_AppsLogDrain_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps log-drain") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsLogDrain_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "log-drain", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps log-drain subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
