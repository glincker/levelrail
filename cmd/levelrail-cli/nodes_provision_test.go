package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesProvision(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createNodeProvisionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(nodeProvisionResource{ID: "npv_1", Provider: gotBody.Provider, Status: "booting"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"nodes", "provision",
		"--provider", "hetzner", "--region", "fsn1", "--size", "cx22", "--name", "web-1",
		"--control-plane-addr", "cp.example.com:9443",
		"--api-url", srv.URL,
	})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/provision" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/provision", gotMethod, gotPath)
	}
	if gotBody.Provider != "hetzner" || gotBody.Region != "fsn1" || gotBody.Size != "cx22" || gotBody.Name != "web-1" || gotBody.ControlPlaneAddr != "cp.example.com:9443" {
		t.Errorf("request body = %+v", gotBody)
	}
	if gotBody.Role != "general" {
		t.Errorf("Role = %q, want the default general", gotBody.Role)
	}
	if !strings.Contains(stdout, "npv_1") || !strings.Contains(stdout, "booting") {
		t.Errorf("stdout = %q, want the provision id and status", stdout)
	}
}

func TestRun_NodesProvision_DerivesControlPlaneAddrFromAPIURL(t *testing.T) {
	var gotBody createNodeProvisionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(nodeProvisionResource{ID: "npv_2", Status: "booting"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"nodes", "provision",
		"--provider", "hetzner", "--region", "fsn1", "--size", "cx22", "--name", "web-2",
		"--api-url", srv.URL,
	})
	host := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	host = strings.SplitN(host, ":", 2)[0]
	if gotBody.ControlPlaneAddr != host+":9443" {
		t.Errorf("ControlPlaneAddr = %q, want %q", gotBody.ControlPlaneAddr, host+":9443")
	}
}

func TestRun_NodesProvision_MissingFlags(t *testing.T) {
	cases := [][]string{
		{"nodes", "provision", "--region", "r", "--size", "s", "--name", "n"},
		{"nodes", "provision", "--provider", "hetzner", "--size", "s", "--name", "n"},
		{"nodes", "provision", "--provider", "hetzner", "--region", "r", "--name", "n"},
		{"nodes", "provision", "--provider", "hetzner", "--region", "r", "--size", "s"},
	}
	for _, args := range cases {
		runCLIExpectValidationError(t, args)
	}
}

func TestRun_NodesProvision_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "provision", "-h"})
	if !strings.Contains(stderr, "nodes provision") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
