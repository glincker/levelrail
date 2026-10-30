package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

const (
	mcpListBody   = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	mcpCallBody   = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_apps","arguments":{"api_key":"sk_live_secretvalue"}}}`
	mcpMutateBody = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"deploy_app","arguments":{"name":"web","image":"nginx"}}}`
)

func mcpPost(_ *Router, h http.Handler, token, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func mcpEnabled(t *testing.T) {
	t.Helper()
	experimental.Set(experimental.MCPHTTP)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
}

func TestMCPHTTP_OffByDefault(t *testing.T) {
	rt, _ := newTestRouter(t)
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{})
	if rec := mcpPost(rt, rt.Handler(), tok, mcpListBody, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("flag off = %d, want 404", rec.Code)
	}
}

func TestMCPHTTP_Unauthorized(t *testing.T) {
	mcpEnabled(t)
	rt, _ := newTestRouter(t)
	h := rt.Handler()
	rec := mcpPost(rt, h, "", mcpListBody, nil)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("no token = %d %v", rec.Code, rec.Header())
	}
	if rec := mcpPost(rt, h, "not-a-real-token", mcpListBody, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d", rec.Code)
	}
}

func TestMCPHTTP_WrongAbility(t *testing.T) {
	mcpEnabled(t)
	rt, _ := newTestRouter(t)
	deployOnly := mintAgent(t, rt, "d", []string{AbilityDeploy}, agentIdentity{})
	if rec := mcpPost(rt, rt.Handler(), deployOnly, mcpListBody, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("token without read = %d, want 403", rec.Code)
	}
}

func TestMCPHTTP_ReadTokenListsReadToolsOnly(t *testing.T) {
	mcpEnabled(t)
	rt, _ := newTestRouter(t)
	h := rt.Handler()
	// A write-capable token still gets the read-only tool set.
	tok := mintAgent(t, rt, "w", []string{AbilityRead, AbilityWrite, AbilityDeploy}, agentIdentity{})
	rec := mcpPost(rt, h, tok, mcpListBody, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"list_apps"`) {
		t.Fatalf("tools/list = %d %s", rec.Code, rec.Body.String())
	}
	for _, mutating := range []string{`"deploy_app"`, `"restart_app"`, `"delete_app"`, `"create_app"`} {
		if strings.Contains(rec.Body.String(), mutating) {
			t.Fatalf("mutating tool %s is exposed", mutating)
		}
	}
	rec = mcpPost(rt, h, tok, mcpMutateBody, nil)
	if strings.Contains(rec.Body.String(), `"isError":false`) || !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("mutating call was not refused: %s", rec.Body.String())
	}
}

func TestMCPHTTP_CallIsAuditedWithoutArgumentValues(t *testing.T) {
	mcpEnabled(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	h := rt.Handler()
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{Name: "remote-bot"})
	if rec := mcpPost(rt, h, tok, mcpCallBody, nil); rec.Code != http.StatusOK {
		t.Fatalf("tools/call = %d %s", rec.Code, rec.Body.String())
	}
	cookieReq := func(path string) *http.Request { return authedRequest(t, cookie, http.MethodGet, path, "") }
	var found bool
	for _, e := range auditEntriesFor(t, rt, cookieReq, "?agent=remote-bot") {
		if e.Action != AuditActionMCPToolCall {
			continue
		}
		found = true
		d := string(e.Detail)
		if !strings.Contains(d, "list_apps") || !strings.Contains(d, "api_key") || strings.Contains(d, "sk_live") {
			t.Fatalf("audit detail = %s", d)
		}
	}
	if !found {
		t.Fatal("no mcp.tool_call audit row")
	}
}

func TestMCPHTTP_RateLimit(t *testing.T) {
	mcpEnabled(t)
	t.Setenv(envMCPHTTPRPM, "2")
	rt, _ := newTestRouter(t)
	h := rt.Handler()
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{})
	other := mintAgent(t, rt, "o", []string{AbilityRead}, agentIdentity{})
	for i := 0; i < 2; i++ {
		if rec := mcpPost(rt, h, tok, mcpListBody, nil); rec.Code != http.StatusOK {
			t.Fatalf("call %d = %d", i, rec.Code)
		}
	}
	if rec := mcpPost(rt, h, tok, mcpListBody, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third call = %d, want 429", rec.Code)
	}
	if rec := mcpPost(rt, h, other, mcpListBody, nil); rec.Code != http.StatusOK {
		t.Fatalf("other token = %d, limit must be per token", rec.Code)
	}
}

func TestMCPHTTP_OversizeBody(t *testing.T) {
	mcpEnabled(t)
	t.Setenv(envMCPHTTPMaxBody, "200")
	rt, _ := newTestRouter(t)
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{})
	big := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"pad":"` + strings.Repeat("a", 1000) + `"}}`
	if rec := mcpPost(rt, rt.Handler(), tok, big, nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize = %d, want 413", rec.Code)
	}
}

func TestMCPHTTP_Origin(t *testing.T) {
	mcpEnabled(t)
	t.Setenv("APP_DASHBOARD_URL", "https://dash.example.com")
	t.Setenv(envMCPHTTPOrigins, "https://connector.example.org")
	rt, _ := newTestRouter(t)
	h := rt.Handler()
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{})
	for _, bad := range []string{"https://evil.example", "null", "http://dash.example.com", "https://dash.example.com.evil.example"} {
		if rec := mcpPost(rt, h, tok, mcpListBody, map[string]string{"Origin": bad}); rec.Code != http.StatusForbidden {
			t.Fatalf("origin %q = %d, want 403", bad, rec.Code)
		}
	}
	for _, good := range []string{"https://dash.example.com", "https://connector.example.org"} {
		if rec := mcpPost(rt, h, tok, mcpListBody, map[string]string{"Origin": good}); rec.Code != http.StatusOK {
			t.Fatalf("origin %q = %d, want 200", good, rec.Code)
		}
	}
}
