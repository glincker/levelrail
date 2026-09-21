package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesSetResources(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDatabaseResourcesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "main", Resources: gotBody.Resources})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "set-resources", "main", "--memory", "512Mi", "--cpu", "0.5", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/main/resources" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/databases/main/resources", gotMethod, gotPath)
	}
	if gotBody.Resources == nil || gotBody.Resources.MemoryBytes != 512*1024*1024 {
		t.Errorf("request body resources = %+v, want 512Mi in bytes", gotBody.Resources)
	}
	if gotBody.Resources.NanoCPUs != 500_000_000 {
		t.Errorf("request body nano_cpus = %d, want 500000000", gotBody.Resources.NanoCPUs)
	}
	if !strings.Contains(stdout, "resource limits applied to database") {
		t.Errorf("stdout = %q, want a confirmation message", stdout)
	}
}

func TestRun_DatabasesSetResources_InvalidMemory(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{"databases", "set-resources", "main", "--memory", "bogus", "--api-url", "http://unused"})
	if !strings.Contains(stderr, "invalid memory value") {
		t.Errorf("stderr = %q, want an invalid-memory error", stderr)
	}
}

func TestRun_DatabasesSetResources_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"database not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"databases", "set-resources", "bogus", "--memory", "256Mi", "--api-url", srv.URL})
	if !strings.Contains(stderr, "database not found") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_DatabasesSetResources_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"databases", "set-resources", "-h"})
	if !strings.Contains(stderr, "databases set-resources") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
