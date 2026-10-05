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

func TestRun_Containers_Stop(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"containers", "stop", "some-other-container", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/system/containers/some-other-container/stop" {
		t.Errorf("request = %s %s, want POST .../system/containers/some-other-container/stop", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "stopped") {
		t.Errorf("stdout = %q, want confirmation it stopped", stdout)
	}
}

func TestRun_Containers_Stop_Managed409(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"container is managed by Levelrail; manage it from its own app page instead"}`)
	stderr := runCLIExpectAPIError(t, []string{"containers", "stop", "levelrail-app-web", "--api-url", srv.URL})
	if !strings.Contains(stderr, "managed by Levelrail") {
		t.Errorf("stderr = %q, want the server's managed-container message", stderr)
	}
}

func TestRun_Containers_Remove(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"containers", "remove", "some-other-container", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/system/containers/some-other-container/remove" {
		t.Errorf("request = %s %s, want POST .../system/containers/some-other-container/remove", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "removed") {
		t.Errorf("stdout = %q, want confirmation it was removed", stdout)
	}
}

func TestRun_Containers_Claim(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "my-app", Image: "postgres:16"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"containers", "claim", "some-other-container", "--as", "my-app", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/system/containers/some-other-container/claim" {
		t.Errorf("request = %s %s, want POST .../system/containers/some-other-container/claim", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"my-app"`) {
		t.Errorf("request body = %q, want the --as name sent through", gotBody)
	}
	if !strings.Contains(stdout, "my-app") {
		t.Errorf("stdout = %q, want the claimed app name", stdout)
	}
}
