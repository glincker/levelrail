package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_APIDocs_Table(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAPISpecResource{
			Version:      1,
			Count:        2,
			ExampleCount: 1,
			Routes: []openAPIRouteResource{
				{Method: "GET", Path: "/api/v1/brand", Ability: "Public", Group: "System", Description: "Branding."},
				{Method: "GET", Path: "/api/v1/apps", Ability: "AbilityRead", Group: "Apps"},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"api-docs", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/openapi.json" {
		t.Errorf("request = %s %s, want GET /api/v1/openapi.json", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "/api/v1/brand") || !strings.Contains(stdout, "/api/v1/apps") {
		t.Errorf("stdout = %q, want both routes listed", stdout)
	}
	if !strings.Contains(stdout, "2 routes, 1 with a worked request/response example.") {
		t.Errorf("stdout = %q, want the coverage summary line", stdout)
	}
}

func TestRun_APIDocs_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAPISpecResource{Count: 1})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"api-docs", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"count": 1`) {
		t.Errorf("stdout = %q, want the spec as JSON", stdout)
	}
}

func TestRun_APIDocs_ServerError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)

	stderr := runCLIExpectAPIError(t, []string{"api-docs", "--api-url", srv.URL})
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_APIDocs_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"api-docs", "-h"})
	if !strings.Contains(stderr, "api-docs") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
