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

func TestRun_NodesSimulate(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(apiclient.PlacementSimulation{
			Placed:   []apiclient.SimulatedPlacement{{Name: "app-1", NodeID: "n1", Zone: "a", Why: "spread"}, {Name: "app-2", Unplaced: "no room"}},
			Unplaced: 1,
		})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "simulate", "--count", "2", "--memory", "512", "--api-url", srv.URL})
	if gotPath != "/api/v1/nodes/placement-simulation" || !strings.Contains(gotQuery, "count=2") || !strings.Contains(gotQuery, "memory_mib=512") {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if !strings.Contains(stdout, "1 placed, 1 refused") || !strings.Contains(stdout, "no room") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_NodesFailure_NeedsOneTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"nodes", "failure"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestStrandedRowsAndErrorNextStep(t *testing.T) {
	var b strings.Builder
	strandedRows(&b, []apiclient.NodeReservation{{Name: "n1", MemoryReservedBytes: 2 << 30, MemoryUsedBytes: 1 << 30, MemoryStrandedBytes: 1 << 30}})
	if !strings.Contains(b.String(), "STRANDED") || !strings.Contains(b.String(), "1.0 GiB") {
		t.Errorf("rows = %q", b.String())
	}
	if errorNextStep(404) == "" || errorNextStep(401) == "" || errorNextStep(500) != "" {
		t.Error("unexpected next steps")
	}
}
