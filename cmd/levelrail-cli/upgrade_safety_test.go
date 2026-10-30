package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func safetyPlan(diskOK bool) apiclient.SelfUpgradePlan {
	db, target, pending := 3040, 3048, 8
	return apiclient.SelfUpgradePlan{
		CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0", NotesAvailable: true, CanApply: true, Steps: []string{"download", "health"},
		DBSchemaVersion: &db, TargetSchemaVersion: &target, PendingMigrations: &pending,
		Disk:            apiclient.SelfUpgradeDisk{Known: true, FreeBytes: 100 << 20, RequiredBytes: 70 << 20, OK: diskOK},
		DowntimeSeconds: 15, Restart: "the control plane stops and starts once",
		Backup: apiclient.SelfUpgradeBackup{WillTake: true, RestoreCommand: "sudo levelrail restore-db /data/control-plane-backups/NAME"},
	}
}

func TestUpgradeDryRunReportsWhatWouldChange(t *testing.T) {
	var started string
	srv := selfUpgradeServer(t, safetyPlan(true), &started, nil)
	out, _ := runCLIExpectOK(t, []string{"upgrade", "--dry-run", "--api-url", srv.URL})
	for _, want := range []string{"v1.0.0 -> v1.1.0", "8 migrations", "3040 -> 3048", "disk: 100 MiB free", "(enough)", "--skip-backup", "about 15s", "restore-db /data/control-plane-backups/NAME"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run missing %q:\n%s", want, out)
		}
	}
	if started != "" {
		t.Fatalf("dry run started an upgrade: %q", started)
	}
}

func TestUpgradeApplyRefusesShortDiskUnlessSkipBackup(t *testing.T) {
	var started string
	srv := selfUpgradeServer(t, safetyPlan(false), &started, nil)
	var outBuf, errBuf bytes.Buffer
	if code := run("levelrail-cli-test", []string{"upgrade", "--apply", "--no-wait", "--api-url", srv.URL}, &outBuf, &errBuf, envMap()); code == exitOK {
		t.Fatalf("apply on a short disk exited 0: %s", outBuf.String())
	}
	if started != "" || !strings.Contains(outBuf.String()+errBuf.String(), "not enough free disk") {
		t.Fatalf("started=%q out=%s err=%s", started, outBuf.String(), errBuf.String())
	}
	runCLIExpectOK(t, []string{"upgrade", "--apply", "--no-wait", "--skip-backup", "--api-url", srv.URL})
	if started == "" {
		t.Fatal("--skip-backup did not start the upgrade")
	}
}

func TestUpgradeStopsWhenBackupFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/updates/preflight":
			_ = json.NewEncoder(w).Encode(apiclient.UpdatePreflight{CurrentVersion: "v1.0.0", UpdateAvailable: true})
		case "/api/v1/system/backups":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"disk full"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	var outBuf, errBuf bytes.Buffer
	code := run("levelrail-cli-test", []string{"upgrade", "--api-url", srv.URL}, &outBuf, &errBuf, envMap())
	if code == exitOK || !strings.Contains(errBuf.String(), "--skip-backup") {
		t.Fatalf("code=%d stderr=%s, want a refusal naming --skip-backup", code, errBuf.String())
	}
	runCLIExpectOK(t, []string{"upgrade", "--skip-backup", "--api-url", srv.URL})
}
