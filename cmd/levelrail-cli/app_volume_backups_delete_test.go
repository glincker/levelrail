package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRun_AppVolumeBackupsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"app-volume-backups", "delete", "web", "data", "bkh_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/volumes/data/backups/bkh_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/apps/web/volumes/data/backups/bkh_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `backup "bkh_1" of web/data deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_AppVolumeBackupsDelete_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"app-volume-backups", "delete", "web", "data", "bkh_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}

func TestRun_AppVolumeBackupsDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"backup not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"app-volume-backups", "delete", "web", "data", "bkh_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "backup not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppVolumeBackupsDelete_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"app-volume-backups", "delete", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an app name, a volume name, and a backup id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppVolumeBackupsDelete_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"app-volume-backups", "delete", "-h"})
	if !strings.Contains(stderr, "app-volume-backups delete") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
