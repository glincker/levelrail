package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func aiControlServer(t *testing.T, gotReqs *[]string, gotBody *apiclient.UpdateAIControlRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotReqs = append(*gotReqs, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(gotBody)
			_ = json.NewEncoder(w).Encode(apiclient.AIControlResource{Mode: gotBody.Mode, AllowedEnvKinds: gotBody.AllowedEnvKinds})
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(apiclient.RevokeAgentTokensResource{Revoked: 3})
		default:
			_ = json.NewEncoder(w).Encode(apiclient.AIControlResource{Mode: "observe", AllowedEnvKinds: []string{"dev", "test"}, AgentTokenCount: 2})
		}
	}))
}

func TestRun_AIControl_Status(t *testing.T) {
	var reqs []string
	var body apiclient.UpdateAIControlRequest
	srv := aiControlServer(t, &reqs, &body)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"ai-control", "status", "--api-url", srv.URL})
	if !strings.Contains(stdout, "mode:         observe") || !strings.Contains(stdout, "agent tokens: 2") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AIControl_SetKeepsKindsWhenOmitted(t *testing.T) {
	var reqs []string
	var body apiclient.UpdateAIControlRequest
	srv := aiControlServer(t, &reqs, &body)
	defer srv.Close()
	runCLIExpectOK(t, []string{"ai-control", "set", "--mode", "operate", "--api-url", srv.URL})
	if body.Mode != "operate" || strings.Join(body.AllowedEnvKinds, ",") != "dev,test" {
		t.Errorf("body = %+v", body)
	}
	runCLIExpectOK(t, []string{"ai-control", "set", "--mode", "off", "--env-kinds", "uat, preview", "--api-url", srv.URL})
	if strings.Join(body.AllowedEnvKinds, ",") != "uat,preview" {
		t.Errorf("body = %+v", body)
	}
}

func TestRun_AIControl_RevokeNeedsYes(t *testing.T) {
	var reqs []string
	var body apiclient.UpdateAIControlRequest
	srv := aiControlServer(t, &reqs, &body)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"ai-control", "revoke-agents", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want usage", got)
	}
	if len(reqs) != 0 {
		t.Errorf("made requests without --yes: %v", reqs)
	}
	out, _ := runCLIExpectOK(t, []string{"ai-control", "revoke-agents", "--yes", "--api-url", srv.URL})
	if !strings.Contains(out, "revoked 3 agent tokens") || reqs[0] != "POST /api/v1/settings/ai-control/revoke-agent-tokens" {
		t.Errorf("out = %q reqs = %v", out, reqs)
	}
}

func TestRun_AIControl_SetRequiresMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"ai-control", "set", "--api-url", "http://unused"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d", got)
	}
}
