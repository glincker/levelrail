package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_NodesJoinTokensListAndRevoke(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]apiclient.PendingJoinToken{{ID: "njt_1", Purpose: "enroll", ExpiresAt: time.Now().Add(time.Minute)}})
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.RevokeJoinTokensResponse{Revoked: 1})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "join-tokens", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/nodes/join-tokens" || !strings.Contains(stdout, "njt_1") {
		t.Errorf("list: %s %s, stdout %q", gotMethod, gotPath, stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"nodes", "revoke-join-token", "njt_1", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/nodes/join-tokens/njt_1" || !strings.Contains(stdout, "revoked 1") {
		t.Errorf("revoke one: %s %s, stdout %q", gotMethod, gotPath, stdout)
	}
	runCLIExpectOK(t, []string{"nodes", "revoke-join-token", "--all", "--api-url", srv.URL})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/nodes/join-tokens" {
		t.Errorf("revoke all: %s %s", gotMethod, gotPath)
	}
}
