package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runSystemPruneCommand starts an httptest server that encodes result as
// the prune API's JSON response, runs "system-prune" with extraArgs, and
// requires exitOK. Returns stdout and the request path/method the server
// observed, for each test's own assertions.
func runSystemPruneCommand(t *testing.T, result systemPruneResult, extraArgs ...string) (stdout, gotPath, gotMethod string) {
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
	args := append([]string{"system-prune"}, extraArgs...)
	args = append(args, "--api-url", srv.URL)
	got := run("levelrail-cli-test", args, &out, &errOut, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, out.String(), errOut.String())
	}
	return out.String(), path, method
}

func TestRun_SystemPrune(t *testing.T) {
	stdout, gotPath, gotMethod := runSystemPruneCommand(t, systemPruneResult{
		ContainersRemoved:        []string{"old-web-1"},
		ContainersReclaimedBytes: 1024,
		ImagesRemoved:            []string{"nginx:old"},
		ImagesReclaimedBytes:     2048,
		VolumesRemoved:           []string{},
		BuildCacheReclaimedBytes: 4096,
	})
	if gotPath != "/api/v1/system/prune" {
		t.Errorf("path = %q, want /api/v1/system/prune", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if !strings.Contains(stdout, "containers removed:    1") {
		t.Errorf("stdout = %q, want the containers removed count reported", stdout)
	}
}

func TestRun_SystemPrune_JSON(t *testing.T) {
	stdout, _, _ := runSystemPruneCommand(t, systemPruneResult{ImagesRemoved: []string{"nginx:old"}, ImagesReclaimedBytes: 2048}, "--json")

	var out systemPruneResult
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout, err)
	}
	if len(out.ImagesRemoved) != 1 || out.ImagesReclaimedBytes != 2048 {
		t.Errorf("out = %+v, want one removed image with 2048 bytes reclaimed", out)
	}
}

func TestRun_SystemPrune_ReportsErrors(t *testing.T) {
	stdout, _, _ := runSystemPruneCommand(t, systemPruneResult{Errors: []string{"prune volumes: permission denied"}})
	if !strings.Contains(stdout, "prune volumes: permission denied") {
		t.Errorf("stdout = %q, want the stage error reported", stdout)
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
