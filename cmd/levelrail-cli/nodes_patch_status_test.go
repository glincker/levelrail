package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatPatchStatusHuman(t *testing.T) {
	tests := []struct {
		name   string
		status nodePatchStatusResource
		want   string
	}{
		{name: "never checked", status: nodePatchStatusResource{Checked: false}, want: "unknown, not checked"},
		{name: "checked, up to date", status: nodePatchStatusResource{Checked: true, Total: 0, Security: 0}, want: "up to date"},
		{name: "one update, no security", status: nodePatchStatusResource{Checked: true, Total: 1, Security: 0}, want: "1 update available"},
		{name: "many updates, no security", status: nodePatchStatusResource{Checked: true, Total: 3, Security: 0}, want: "3 updates available"},
		{name: "many updates, one security", status: nodePatchStatusResource{Checked: true, Total: 3, Security: 1}, want: "3 updates available, 1 security"},
		{name: "many updates, many security", status: nodePatchStatusResource{Checked: true, Total: 5, Security: 2}, want: "5 updates available, 2 security"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPatchStatusHuman(tt.status); got != tt.want {
				t.Errorf("formatPatchStatusHuman() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRun_NodesPatchStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodePatchStatusResource{Checked: true, Total: 3, Security: 1})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "patch-status", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/nodes/nd_1/patch-status" {
		t.Errorf("request = %s %s, want GET /api/v1/nodes/nd_1/patch-status", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "3 updates available, 1 security") {
		t.Errorf("stdout = %q, want the human-readable status", stdout)
	}
}

func TestRun_NodesPatchStatus_Unchecked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodePatchStatusResource{Checked: false})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "patch-status", "nd_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "unknown, not checked") {
		t.Errorf("stdout = %q, want the unknown message", stdout)
	}
}

func TestRun_NodesPatchStatus_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodePatchStatusResource{Checked: true, Total: 2})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "patch-status", "nd_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"total": 2`) {
		t.Errorf("stdout = %q, want the status as JSON", stdout)
	}
}

func TestRun_NodesPatchStatus_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "patch-status", "nd_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesPatchStatus_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "patch-status")
}

func TestRun_NodesPatchStatus_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "patch-status", "-h"})
	if !strings.Contains(stderr, "nodes patch-status") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
