package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stdioGet(rt *Router, token string, hdr map[string]string) int {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, r)
	return rec.Code
}

func TestMCPStdioTaggedReadIsAudited(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.UpdateAIControlSettings(context.Background(), "operate", []string{"custom"}, "test"); err != nil {
		t.Fatal(err)
	}
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{Name: "stdio-bot"})
	tag := map[string]string{mcpToolTagHeader: "list_apps", mcpArgsTagHeader: "team,limit"}
	if code := stdioGet(rt, tok, tag); code != http.StatusOK {
		t.Fatalf("tagged read = %d", code)
	}
	// An untagged read stays unaudited, as before.
	if code := stdioGet(rt, tok, nil); code != http.StatusOK {
		t.Fatalf("untagged read = %d", code)
	}
	cookieReq := func(path string) *http.Request { return authedRequest(t, cookie, http.MethodGet, path, "") }
	var rows int
	for _, e := range auditEntriesFor(t, rt, cookieReq, "?agent=stdio-bot") {
		if e.Action != AuditActionMCPToolCall {
			continue
		}
		rows++
		d := string(e.Detail)
		if !strings.Contains(d, "list_apps") || !strings.Contains(d, "limit") || !strings.Contains(d, "stdio") {
			t.Fatalf("detail = %s", d)
		}
	}
	if rows != 1 {
		t.Fatalf("mcp.tool_call rows = %d, want 1", rows)
	}
}
