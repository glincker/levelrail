package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runVolumesOrphanedCommand(t *testing.T, result []orphanedVolumeResource, extraArgs ...string) (stdout, gotPath, gotMethod string) {
	t.Helper()
	var path, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		method = r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	args := append([]string{"volumes-orphaned"}, extraArgs...)
	args = append(args, "--api-url", srv.URL)
	got := run("levelrail-cli-test", args, &out, &errOut, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, out.String(), errOut.String())
	}
	return out.String(), path, method
}

func TestRun_VolumesOrphaned(t *testing.T) {
	size := int64(1024)
	stdout, gotPath, gotMethod := runVolumesOrphanedCommand(t, []orphanedVolumeResource{
		{Name: "app-old-web-data", SizeBytes: &size, CreatedAt: "2026-01-01T00:00:00Z"},
	})
	if gotPath != "/api/v1/system/volumes/orphaned" {
		t.Errorf("path = %q, want /api/v1/system/volumes/orphaned", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if !strings.Contains(stdout, "app-old-web-data") {
		t.Errorf("stdout = %q, want the orphaned volume name reported", stdout)
	}
	if !strings.Contains(stdout, "1024") {
		t.Errorf("stdout = %q, want the size reported", stdout)
	}
}

func TestRun_VolumesOrphaned_Empty(t *testing.T) {
	stdout, _, _ := runVolumesOrphanedCommand(t, []orphanedVolumeResource{})
	if !strings.Contains(stdout, "no orphaned volumes") {
		t.Errorf("stdout = %q, want a clear empty-state message", stdout)
	}
}

func TestRun_VolumesOrphaned_JSON(t *testing.T) {
	size := int64(2048)
	stdout, _, _ := runVolumesOrphanedCommand(t, []orphanedVolumeResource{{Name: "app-a", SizeBytes: &size}}, "--json")

	var out []orphanedVolumeResource
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout, err)
	}
	if len(out) != 1 || out[0].Name != "app-a" {
		t.Errorf("out = %+v, want one volume named app-a", out)
	}
}

func TestRun_VolumesOrphaned_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"volumes-orphaned", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "volumes-orphaned [flags]") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func runVolumesOrphanedCleanupCommand(t *testing.T, result cleanupOrphanedVolumesResult, extraArgs ...string) (stdout, gotPath, gotMethod, gotBody string, exitCode int) {
	t.Helper()
	var path, method, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		method = r.Method
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		body = buf.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	args := append([]string{"volumes-orphaned-cleanup"}, extraArgs...)
	args = append(args, "--api-url", srv.URL)
	exitCode = run("levelrail-cli-test", args, &out, &errOut, envMap())
	return out.String(), path, method, body, exitCode
}

func TestRun_VolumesOrphanedCleanup_Success(t *testing.T) {
	stdout, gotPath, gotMethod, gotBody, exitCode := runVolumesOrphanedCleanupCommand(t,
		cleanupOrphanedVolumesResult{Removed: []string{"app-old-a", "app-old-b"}, ReclaimedBytes: 4096},
		"--names", "app-old-a,app-old-b",
	)
	if exitCode != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q)", exitCode, exitOK, stdout)
	}
	if gotPath != "/api/v1/system/volumes/orphaned/cleanup" {
		t.Errorf("path = %q, want /api/v1/system/volumes/orphaned/cleanup", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.Contains(gotBody, "app-old-a") || !strings.Contains(gotBody, "app-old-b") {
		t.Errorf("request body = %q, want both names sent", gotBody)
	}
	if !strings.Contains(stdout, "removed:   2") {
		t.Errorf("stdout = %q, want the removed count reported", stdout)
	}
}

func TestRun_VolumesOrphanedCleanup_ReportsSkippedAndErrors(t *testing.T) {
	stdout, _, _, _, exitCode := runVolumesOrphanedCleanupCommand(t,
		cleanupOrphanedVolumesResult{
			Removed: []string{"app-ok"},
			Skipped: []string{"app-still-live"},
			Errors:  []string{"app-fails: daemon busy"},
		},
		"--names", "app-ok,app-still-live,app-fails",
	)
	if exitCode != exitOK {
		t.Fatalf("exit = %d, want %d", exitCode, exitOK)
	}
	if !strings.Contains(stdout, "app-still-live") {
		t.Errorf("stdout = %q, want the skipped volume reported", stdout)
	}
	if !strings.Contains(stdout, "daemon busy") {
		t.Errorf("stdout = %q, want the removal error reported", stdout)
	}
}

func TestRun_VolumesOrphanedCleanup_RequiresNames(t *testing.T) {
	var out, errOut bytes.Buffer
	got := run("levelrail-cli-test", []string{"volumes-orphaned-cleanup", "--api-url", "http://unused.invalid"}, &out, &errOut, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want a non-zero exit when --names is missing (stdout=%q stderr=%q)", got, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "--names is required") && !strings.Contains(errOut.String(), "--names is required") {
		t.Errorf("output = stdout:%q stderr:%q, want a clear --names-required message", out.String(), errOut.String())
	}
}

func TestRun_VolumesOrphanedCleanup_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"volumes-orphaned-cleanup", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "volumes-orphaned-cleanup --names") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
