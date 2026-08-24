package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesWorkloads(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setNodeWorkloadsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeResource{
			ID: "nd_1", Name: "web-1",
			AcceptsAppWorkloads: gotBody.AcceptsAppWorkloads, AcceptsBuildWorkloads: gotBody.AcceptsBuildWorkloads,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"nodes", "workloads", "nd_1", "--accepts-app=true", "--accepts-build=false", "--api-url", srv.URL,
	})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/nodes/nd_1/workloads" {
		t.Errorf("request = %s %s, want PUT /api/v1/nodes/nd_1/workloads", gotMethod, gotPath)
	}
	if !gotBody.AcceptsAppWorkloads || gotBody.AcceptsBuildWorkloads {
		t.Errorf("request body = %+v, want accepts_app_workloads=true accepts_build_workloads=false", gotBody)
	}
	if !strings.Contains(stdout, "accepts_app_workloads=true") || !strings.Contains(stdout, "accepts_build_workloads=false") {
		t.Errorf("stdout = %q, want both workload flags echoed", stdout)
	}
}

func TestRun_NodesWorkloads_ExplicitFalse(t *testing.T) {
	var gotBody setNodeWorkloadsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeResource{ID: "nd_1"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"nodes", "workloads", "nd_1", "--accepts-app=false", "--accepts-build=false", "--api-url", srv.URL,
	})
	if gotBody.AcceptsAppWorkloads || gotBody.AcceptsBuildWorkloads {
		t.Errorf("request body = %+v, want both explicitly false", gotBody)
	}
}

func TestRun_NodesWorkloads_MissingAcceptsApp(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"nodes", "workloads", "nd_1", "--accepts-build=true"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--accepts-app is required") {
		t.Errorf("stderr = %q, want a missing --accepts-app error", stderr.String())
	}
}

func TestRun_NodesWorkloads_MissingAcceptsBuild(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"nodes", "workloads", "nd_1", "--accepts-app=true"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--accepts-build is required") {
		t.Errorf("stderr = %q, want a missing --accepts-build error", stderr.String())
	}
}

func TestRun_NodesWorkloads_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{
		"nodes", "workloads", "nd_missing", "--accepts-app=true", "--accepts-build=true", "--api-url", srv.URL,
	})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesWorkloads_NoID(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"nodes", "workloads", "--accepts-app=true", "--accepts-build=true"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-id usage error", stderr.String())
	}
}

func TestRun_NodesWorkloads_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "workloads", "-h"})
	if !strings.Contains(stderr, "nodes workloads") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
