package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestRun_Doctor_PrintsFixCommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemDoctorResource{
			OK: true,
			Checks: []doctorCheckResource{
				{Code: "clock_skew", Name: "Clock skew", Status: "warn", Message: "off by 1h", Fix: "sudo systemctl enable --now systemd-timesyncd"},
				{Code: "docker", Name: "Docker daemon", Status: "ok", Message: "reachable"},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"doctor", "--api-url", srv.URL})
	if !strings.Contains(stdout, "Fixes:") {
		t.Errorf("stdout = %q, want a Fixes section", stdout)
	}
	if !strings.Contains(stdout, "sudo systemctl enable --now systemd-timesyncd") {
		t.Errorf("stdout = %q, want the clock_skew fix command", stdout)
	}
	if strings.Count(stdout, "Docker daemon") != 1 {
		t.Errorf("stdout = %q, want Docker daemon (which has no Fix) only in the table, not the Fixes section", stdout)
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

func TestRun_Doctor_LocalOnlyWithoutServer(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"doctor", "--local", "--output", "json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, stderr=%q", got, stderr.String())
	}
	var rep systemDoctorResource
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v: %s", err, stdout.String())
	}
	want := map[string]bool{"docker": true, "disk_space": true, "port_80": true, "port_443": true}
	for _, c := range rep.Checks {
		delete(want, c.Code)
	}
	if len(want) != 0 {
		t.Errorf("local report is missing checks %v", want)
	}
}

func TestRun_Doctor_FixNeedsServer(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"doctor", "--local", "--fix"}, &stdout, &stderr, envMap()); got == exitOK {
		t.Fatalf("--fix with no server exited 0, stdout=%q", stdout.String())
	}
}

type doctorFixServer struct {
	mu      sync.Mutex
	applied []string
	fixed   bool
}

func (s *doctorFixServer) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/api/v1/system/doctor":
		status, fixID, kind := "warn", "disk.reclaim", "auto"
		if s.fixed {
			status, fixID, kind = "ok", "", ""
		}
		_ = json.NewEncoder(w).Encode(systemDoctorResource{OK: true, Checks: []doctorCheckResource{
			{Code: "disk_space", Name: "Disk space", Status: status, Message: "low", FixID: fixID, FixKind: kind},
			{Code: "firewall", Name: "Firewall", Status: "warn", Message: "blocked", FixID: "host.firewall.open_web_ports", FixKind: "guided"},
			{Code: "dashboard_url_https", Name: "Dashboard https", Status: "warn", Message: "http", FixID: "settings.dashboard_url.https", FixKind: "auto"},
		}})
	case strings.HasSuffix(r.URL.Path, "/preview"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/fixes/"), "/preview")
		f := map[string]any{"id": id, "title": id, "kind": "auto", "risk": "safe", "state": "offer", "failing": true, "changes": []any{}, "attempts": []any{}}
		switch id {
		case "host.firewall.open_web_ports":
			f["kind"] = "guided"
		case "settings.dashboard_url.https":
			f["risk"] = "exposes"
		}
		_ = json.NewEncoder(w).Encode(f)
	case strings.HasSuffix(r.URL.Path, "/apply"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/fixes/"), "/apply")
		s.applied = append(s.applied, id)
		s.fixed = true
		_ = json.NewEncoder(w).Encode(map[string]any{"fix_id": id, "outcome": "fixed", "detail": "check passes"})
	default:
		http.NotFound(w, r)
	}
}

func TestRun_Doctor_FixAppliesOnlySafeAutoFixes(t *testing.T) {
	fs := &doctorFixServer{}
	srv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"doctor", "--fix", "--yes", "--output", "json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, stderr=%q", got, stderr.String())
	}
	if len(fs.applied) != 1 || fs.applied[0] != "disk.reclaim" {
		t.Errorf("applied = %v, want only disk.reclaim (guided and exposing fixes are never auto applied)", fs.applied)
	}
	var out doctorOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v: %s", err, stdout.String())
	}
	if len(out.Applied) != 1 || len(out.Skipped) != 2 {
		t.Errorf("applied=%d skipped=%d, want 1 and 2", len(out.Applied), len(out.Skipped))
	}
	if out.Checks[0].Status != "ok" {
		t.Errorf("report after fixing still shows disk_space %q", out.Checks[0].Status)
	}
}

func TestRun_Doctor_FixWithoutYesNeedsTerminal(t *testing.T) {
	fs := &doctorFixServer{}
	srv := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"doctor", "--fix", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("--fix without --yes on a non-terminal exited 0")
	}
	if len(fs.applied) != 0 {
		t.Errorf("applied %v without confirmation", fs.applied)
	}
}
