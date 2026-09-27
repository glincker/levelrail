package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_TokensCreate_AgentLabel(t *testing.T) {
	var gotReq createTokenRequest
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createTokenResponse{tokenResource: tokenResource{ID: "tok_a", Name: gotReq.Name, Agent: gotReq.Agent, CreatedAt: time.Now()}, Token: "v"})
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "create", "--name", "ci", "--abilities", "read", "--agent", "deploy-bot", "--agent-description", "ships main", "--username", "admin", "--password", "x", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	if gotReq.Agent == nil || gotReq.Agent.Name != "deploy-bot" || gotReq.Agent.Description != "ships main" {
		t.Errorf("agent not forwarded: %+v", gotReq.Agent)
	}

	stdout.Reset()
	stderr.Reset()
	got = run("levelrail-cli-test", []string{"tokens", "create", "--name", "ci", "--abilities", "read", "--agent-description", "orphan", "--username", "admin", "--password", "x", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Errorf("--agent-description without --agent exit = %d, want %d", got, exitValidation)
	}
}

func TestRun_AuditLog_AgentFilterAndColumn(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]auditLogEntryResource{{ID: "a1", ActorName: "ci", ActorType: "token", AgentName: "deploy-bot", AgentClient: "claude-code/2.1", Method: "POST", Path: "/x", StatusCode: 200}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--agent", "deploy-bot", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(gotQuery, "agent=deploy-bot") {
		t.Errorf("query = %q, want agent forwarded", gotQuery)
	}
	if !strings.Contains(stdout.String(), "deploy-bot via claude-code/2.1") {
		t.Errorf("table lacks the agent column: %s", stdout.String())
	}
}
