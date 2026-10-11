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

func TestRun_BackupsHealth(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"resources":[{"kind":"volume","app_name":"web","resource_name":"data","backup_count":2,"total_bytes":2048,"encrypted":true,"state":"unverified","state_reason":"no restore has been proven to work yet","last_backup":{"id":"b1","at":"2026-10-01T00:00:00Z","status":"succeeded","size_bytes":1024}}],"targets":[{"target_id":"bkt_1","level":"open","can_delete":true,"warning":"The backup key can delete or overwrite stored backups","checked_at":"2026-10-01T00:00:00Z"}],"encryption":{"enabled":true,"codec":"zstd+age"}}`)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "health", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/backups/health" {
		t.Errorf("path = %q", gotPath)
	}
	for _, want := range []string{"web/data", "unverified", "never", "2026-10-01T00:00:00Z", "bkt_1", "can delete or overwrite"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func drillServer(t *testing.T, status, errMsg string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/backups/drills":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["backup_id"] != "bkh_1" {
				http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"bkd_1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/backups/drills/bkd_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "bkd_1", "backup_id": "bkh_1", "status": status, "stage": "restore", "error": errMsg, "object_ok": true, "checksum_ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRun_BackupsDrillRun(t *testing.T) {
	cases := []struct {
		name, status, errMsg string
		wantExit             int
		wantOut              string
	}{
		{"passed", "passed", "", exitOK, "passed"},
		{"failed drill exits non zero", "failed", "tar: short read", exitCheckFailed, "tar: short read"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := drillServer(t, c.status, c.errMsg)
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"backups", "drill", "run", "--backup", "bkh_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
			if got != c.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, c.wantExit, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), c.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), c.wantOut)
			}
		})
	}
}

func TestRun_BackupsDrillRun_RequiresBackup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "drill", "run", "--api-url", "http://127.0.0.1:1"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
}

func TestRun_BackupsVolumesRestoreAndPolicy(t *testing.T) {
	var restoreBody, policyBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/volumes/data/restore-to":
			_ = json.NewDecoder(r.Body).Decode(&restoreBody)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"vcr_1","new_volume_name":"app-web-copy-data","node_id":"n2"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web/volumes/data/backup-policy":
			_ = json.NewDecoder(r.Body).Decode(&policyBody)
			_ = json.NewEncoder(w).Encode(policyBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "volumes", "restore", "web", "data", "--backup", "bkh_1", "--target-app", "web-copy", "--node", "n2", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("restore exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if restoreBody["backup_id"] != "bkh_1" || restoreBody["target_app"] != "web-copy" || restoreBody["node_id"] != "n2" {
		t.Errorf("restore body = %v", restoreBody)
	}
	if !strings.Contains(stdout.String(), "app-web-copy-data") {
		t.Errorf("stdout = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	got = run("levelrail-cli-test", []string{"backups", "volumes", "policy", "set", "web", "data", "--retain-daily", "7", "--retain-monthly", "6", "--pause", "--pre-hook", "sync", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("policy exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if policyBody["retain_daily"] != float64(7) || policyBody["retain_monthly"] != float64(6) || policyBody["quiesce"] != "pause" || policyBody["pre_hook"] != "sync" {
		t.Errorf("policy body = %v", policyBody)
	}
}
