package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SystemPrune(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemPruneResult{
			ContainersRemoved:        []string{"old-web-1"},
			ContainersReclaimedBytes: 1024,
			ImagesRemoved:            []string{"nginx:old"},
			ImagesReclaimedBytes:     2048,
			VolumesRemoved:           []string{},
			BuildCacheReclaimedBytes: 4096,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"system-prune", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/system/prune" {
		t.Errorf("path = %q, want /api/v1/system/prune", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.Contains(stdout.String(), "containers removed:    1") {
		t.Errorf("stdout = %q, want the containers removed count reported", stdout.String())
	}
}

func TestRun_SystemPrune_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemPruneResult{ImagesRemoved: []string{"nginx:old"}, ImagesReclaimedBytes: 2048})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"system-prune", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var out systemPruneResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if len(out.ImagesRemoved) != 1 || out.ImagesReclaimedBytes != 2048 {
		t.Errorf("out = %+v, want one removed image with 2048 bytes reclaimed", out)
	}
}

func TestRun_SystemPrune_ReportsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(systemPruneResult{Errors: []string{"prune volumes: permission denied"}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"system-prune", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (a partial stage failure is still a 200, not a CLI error)", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "prune volumes: permission denied") {
		t.Errorf("stdout = %q, want the stage error reported", stdout.String())
	}
}

func TestRun_SystemPrune_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"system-prune", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "system-prune [flags]") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
