package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsPlacementExplainsDecision(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"app","name":"web","node_id":"node_b","recorded":{"trigger":"create","decided_at":"2026-10-16T10:00:00Z","decision":{"node_id":"node_b","why":"most headroom","considered":[
{"node_id":"node_a","eligible":false,"reason":"insufficient capacity: memory: needs 2.0 GiB, 1.0 GiB free of 1.0 GiB allocatable (short by 1.0 GiB)","dimensions":[]},
{"node_id":"node_b","eligible":true,"dimensions":[{"name":"memory","allocatable":8589934592,"requested":0,"need":0,"free":8589934592,"shortfall":0}]}]}},
"preview":{"node_id":"node_b","why":"most headroom","considered":[]}}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "placement", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/placement/decision" {
		t.Errorf("path = %q", gotPath)
	}
	for _, want := range []string{"runs on node_b", "why: most headroom", "refused", "short by 1.0 GiB", "fits"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_NodesAllocations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"policy":{"cpu_overcommit_percent":100,"memory_overcommit_percent":150,"min_free_disk_percent":10,"reject_unknown":false},"nodes":[
{"node_id":"n1","name":"alpha","workloads":2,"dimensions":[{"name":"memory","allocatable":2147483648,"requested":1073741824,"need":0,"free":1073741824,"shortfall":0},{"name":"cpu","allocatable":null,"requested":0,"need":0,"free":null,"shortfall":0}]}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "allocations", "--api-url", srv.URL})
	for _, want := range []string{"memory 150%", "alpha", "memory 1.0 GiB/2.0 GiB", "cpu unknown"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}
