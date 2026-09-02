package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_Containers_ListsRunningAndStopped(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]containerResource{
			{Name: "levelrail-app-web", Image: "levelrail/web:1", Running: true, Ports: []containerPortResource{{ContainerPort: 3000, HostPort: 33001, Protocol: "tcp"}}},
			{Name: "some-other-container", Image: "postgres:16", Running: false},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"containers", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/system/containers" {
		t.Errorf("request = %s %s, want GET /api/v1/system/containers", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "levelrail-app-web") || !strings.Contains(stdout, "running") {
		t.Errorf("stdout = %q, want the running container listed", stdout)
	}
	if !strings.Contains(stdout, "some-other-container") || !strings.Contains(stdout, "stopped") {
		t.Errorf("stdout = %q, want the stopped, non-Levelrail container listed too", stdout)
	}
	if !strings.Contains(stdout, "33001->3000/tcp") {
		t.Errorf("stdout = %q, want the port mapping formatted", stdout)
	}
}

func TestRun_Containers_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]containerResource{{Name: "web", Image: "levelrail/web:1", Running: true}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"containers", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"name": "web"`) {
		t.Errorf("stdout = %q, want the container as JSON", stdout)
	}
}

func TestRun_Containers_NotConfigured(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotImplemented, `{"error":"container listing is not configured on this control plane"}`)
	stderr := runCLIExpectAPIError(t, []string{"containers", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not configured") {
		t.Errorf("stderr = %q, want the server's not-configured message", stderr)
	}
}
