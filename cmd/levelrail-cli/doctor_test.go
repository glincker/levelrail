package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Doctor_AllOK(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemDoctorResource{
			OK: true,
			Checks: []doctorCheckResource{
				{Code: "docker", Name: "Docker daemon", Status: "ok", Message: "reachable"},
				{Code: "database", Name: "Control plane database", Status: "ok", Message: "reachable"},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"doctor", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/system/doctor" {
		t.Errorf("request = %s %s, want GET /api/v1/system/doctor", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "OK") || !strings.Contains(stdout, "Docker daemon") {
		t.Errorf("stdout = %q, want an OK row for Docker daemon", stdout)
	}
	if !strings.Contains(stdout, "All checks passed.") {
		t.Errorf("stdout = %q, want the all-ok summary line", stdout)
	}
}

func TestRun_Doctor_CheckFailing_ExitsNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemDoctorResource{
			OK: false,
			Checks: []doctorCheckResource{
				{Code: "docker", Name: "Docker daemon", Status: "fail", Message: "daemon unreachable"},
			},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"doctor", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (any check failing), stdout=%q", got, exitUsage, stdout.String())
	}
	if !strings.Contains(stdout.String(), "FAIL") || !strings.Contains(stdout.String(), "daemon unreachable") {
		t.Errorf("stdout = %q, want a FAIL row with the daemon error", stdout.String())
	}
	if !strings.Contains(stdout.String(), "One or more checks failed.") {
		t.Errorf("stdout = %q, want the failure summary line", stdout.String())
	}
}

func TestRun_Doctor_WarnDoesNotAffectExitCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemDoctorResource{
			OK: true,
			Checks: []doctorCheckResource{
				{Code: "disk_space", Name: "Disk space", Status: "warn", Message: "low on space"},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"doctor", "--api-url", srv.URL})
	if !strings.Contains(stdout, "WARN") {
		t.Errorf("stdout = %q, want a WARN row", stdout)
	}
}

func TestRun_Doctor_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemDoctorResource{OK: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"doctor", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"ok": true`) {
		t.Errorf("stdout = %q, want the doctor report as JSON", stdout)
	}
}

func TestRun_Doctor_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	stderr := runCLIExpectAPIError(t, []string{"doctor", "--api-url", srv.URL})
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_Doctor_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"doctor", "-h"})
	if !strings.Contains(stderr, "doctor") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
