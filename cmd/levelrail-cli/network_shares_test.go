package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NetworkSharesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []networkShareResource{
		{ID: "ns_1", Name: "backups-nas", Protocol: "nfs", Host: "nas.internal", RemotePath: "/exports/data", CreatedAt: "2026-10-01T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"network-shares", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/network-shares" {
		t.Errorf("path = %q, want /api/v1/network-shares", gotPath)
	}
	if !strings.Contains(stdout, "ns_1") || !strings.Contains(stdout, "backups-nas") {
		t.Errorf("stdout = %q, want the share listed", stdout)
	}
}

func TestRun_NetworkSharesList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []networkShareResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"network-shares", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no network shares") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

func TestRun_NetworkSharesCreate(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(networkShareResource{ID: "ns_new", Name: "backups-nas", Protocol: "nfs", Host: "nas.internal", RemotePath: "/exports/data"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"network-shares", "create",
		"--name", "backups-nas", "--protocol", "nfs", "--host", "nas.internal", "--remote-path", "/exports/data",
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/network-shares" {
		t.Errorf("request = %s %s, want POST /api/v1/network-shares", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"nfs"`) || !strings.Contains(gotBody, `"nas.internal"`) {
		t.Errorf("request body = %q, want protocol/host sent through", gotBody)
	}
	if !strings.Contains(stdout, "ns_new") {
		t.Errorf("stdout = %q, want the created share id", stdout)
	}
}

func TestRun_NetworkSharesCreate_MissingProtocol(t *testing.T) {
	stderr := runCLIExpectValidationError(t, []string{
		"network-shares", "create", "--name", "x", "--host", "h", "--remote-path", "/p",
	})
	if !strings.Contains(stderr, "--protocol is required") {
		t.Errorf("stderr = %q, want a --protocol required error", stderr)
	}
}

func TestRun_NetworkSharesUpdate(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(networkShareResource{ID: "ns_1", Name: "renamed", Protocol: "cifs", Host: "nas.internal", RemotePath: "/share"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"network-shares", "update", "ns_1",
		"--name", "renamed", "--protocol", "cifs", "--host", "nas.internal", "--remote-path", "/share",
		"--username", "bot", "--password", "newpass",
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/network-shares/ns_1" {
		t.Errorf("request = %s %s, want PUT /api/v1/network-shares/ns_1", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"newpass"`) {
		t.Errorf("request body = %q, want the rotated password sent through", gotBody)
	}
	if !strings.Contains(stdout, "renamed") {
		t.Errorf("stdout = %q, want the updated name", stdout)
	}
}

func TestRun_NetworkSharesDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"network-shares", "delete", "ns_1", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/network-shares/ns_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/network-shares/ns_1", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "disconnected") {
		t.Errorf("stdout = %q, want confirmation", stdout)
	}
}

func TestRun_NetworkSharesTest_Unreachable(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadGateway, `{"error":"could not reach nas.internal:2049: dial failed"}`)
	stderr := runCLIExpectAPIError(t, []string{"network-shares", "test", "ns_1", "--api-url", srv.URL})
	if !strings.Contains(stderr, "could not reach") {
		t.Errorf("stderr = %q, want the server's unreachable message", stderr)
	}
}

func TestRun_NetworkSharesTest_OK(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"network-shares", "test", "ns_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/network-shares/ns_1/test" {
		t.Errorf("request = %s %s, want POST /api/v1/network-shares/ns_1/test", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "reachable") {
		t.Errorf("stdout = %q, want confirmation", stdout)
	}
}
