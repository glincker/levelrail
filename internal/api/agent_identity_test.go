package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mintAgent(t *testing.T, rt *Router, name string, abilities []string, agent agentIdentity) string {
	t.Helper()
	plain, _, err := MintAgentAPIToken(context.Background(), rt.tokens, name, abilities, nil, agent)
	if err != nil {
		t.Fatalf("mint %s: %v", name, err)
	}
	return plain
}

func agentBearerRequest(method, path, token, client string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+token)
	if client != "" {
		r.Header.Set(AgentClientHeader, client)
	}
	return r
}

func auditEntriesFor(t *testing.T, rt *Router, cookieReq func(string) *http.Request, query string) []auditLogEntryResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, cookieReq("/api/v1/audit-log"+query))
	if rec.Code != http.StatusOK {
		t.Fatalf("audit-log%s = %d: %s", query, rec.Code, rec.Body.String())
	}
	var out []auditLogEntryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAgentTokens_ScopeAndAudit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cookieReq := func(path string) *http.Request { return authedRequest(t, cookie, http.MethodGet, path, "") }

	reader := mintAgent(t, rt, "reader", []string{AbilityRead}, agentIdentity{Name: "reader-bot", Description: "read only"})
	deployer := mintAgent(t, rt, "deployer", []string{AbilityRead, AbilityDeploy}, agentIdentity{Name: "deploy-bot"})
	plain := mintAgent(t, rt, "plain", []string{AbilityRead, AbilityDeploy}, agentIdentity{})

	const deployPath = "/api/v1/apps/web/deploys"

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, agentBearerRequest(http.MethodPost, deployPath, reader, "claude-code/2.1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only agent token deploy = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, agentBearerRequest(http.MethodPost, deployPath, deployer, "claude-code/2.1\x00"))
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Fatalf("deploy-scoped agent token was rejected: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, agentBearerRequest(http.MethodPost, deployPath, plain, ""))

	entries := auditEntriesFor(t, rt, cookieReq, "?agent=deploy-bot")
	if len(entries) != 1 {
		t.Fatalf("agent=deploy-bot entries = %d, want 1: %+v", len(entries), entries)
	}
	if e := entries[0]; e.AgentName != "deploy-bot" || e.AgentClient != "claude-code/2.1" || e.ActorType != "token" || e.Path != deployPath {
		t.Errorf("audit entry = %+v", e)
	}
	if got := auditEntriesFor(t, rt, cookieReq, "?agent=reader-bot"); len(got) != 0 {
		t.Errorf("the rejected read-only agent must leave no audit entry, got %+v", got)
	}
	all := auditEntriesFor(t, rt, cookieReq, "")
	for _, e := range all {
		if e.ActorName == "plain" && (e.AgentName != "" || e.AgentClient != "") {
			t.Errorf("unlabeled token entry carries agent identity: %+v", e)
		}
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/tokens", ""))
	var tokens []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	var labeled int
	for _, tk := range tokens {
		if tk.Agent != nil && tk.Agent.Name != "" {
			labeled++
		}
	}
	if labeled != 2 {
		t.Errorf("token list shows %d agent labels, want 2: %+v", labeled, tokens)
	}
}

func TestCreateToken_AgentValidation(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"labeled", `{"name":"t1","abilities":["read"],"agent":{"name":"ci-bot","description":"d"}}`, http.StatusCreated},
		{"unlabeled", `{"name":"t2","abilities":["read"]}`, http.StatusCreated},
		{"agent without name", `{"name":"t3","abilities":["read"],"agent":{"description":"d"}}`, http.StatusBadRequest},
		{"agent name too long", `{"name":"t4","abilities":["read"],"agent":{"name":"` + strings.Repeat("a", 65) + `"}}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", tc.body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusCreated && strings.Contains(tc.body, "ci-bot") {
				var got createTokenResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Agent == nil || got.Agent.Name != "ci-bot" {
					t.Errorf("response = %s (%v)", rec.Body.String(), err)
				}
			}
		})
	}
}
