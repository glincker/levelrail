package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesProvidersConnectTestDisconnect(t *testing.T) {
	var reqs []string
	var gotBody setNodeProviderCredentialRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/test"):
			_ = json.NewEncoder(w).Encode(nodeProviderTestResult{OK: true, Message: "Connected"})
		default:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_ = json.NewEncoder(w).Encode(nodeProviderResource{Provider: gotBody.Provider, HasToken: true})
		}
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"nodes", "providers", "connect", "--provider", "custom", "--label", "Rack 4", "--method", "manual", "--api-url", srv.URL})
	if gotBody.Provider != "custom" || gotBody.Method != "manual" || gotBody.Label != "Rack 4" || gotBody.Verify {
		t.Errorf("connect body = %+v", gotBody)
	}
	out, _ := runCLIExpectOK(t, []string{"nodes", "providers", "test", "--provider", "hetzner", "--api-url", srv.URL})
	if !strings.Contains(out, "Connected") {
		t.Errorf("test stdout = %q", out)
	}
	runCLIExpectOK(t, []string{"nodes", "providers", "disconnect", "--provider", "hetzner", "--api-url", srv.URL})
	want := []string{"POST /api/v1/node-providers", "POST /api/v1/node-providers/hetzner/test", "DELETE /api/v1/node-providers/hetzner"}
	if strings.Join(reqs, "|") != strings.Join(want, "|") {
		t.Errorf("requests = %v, want %v", reqs, want)
	}
}
