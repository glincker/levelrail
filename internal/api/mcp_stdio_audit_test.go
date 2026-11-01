package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func stdioReq(rt *Router, method, path, auth string, hdr map[string]string) int {
	r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	if auth != "" {
		r.Header.Set("Authorization", "Bearer "+auth)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, r)
	return rec.Code
}

func stdioRows(t *testing.T, rt *Router, cookie *http.Cookie, agent string) (tagRows int, all []auditLogEntryResource) {
	t.Helper()
	cookieReq := func(path string) *http.Request { return authedRequest(t, cookie, http.MethodGet, path, "") }
	all = auditEntriesFor(t, rt, cookieReq, "?agent="+agent)
	for _, e := range all {
		if e.Action == AuditActionMCPToolCall {
			tagRows++
		}
	}
	return tagRows, all
}

func TestMCPStdioTaggedWriteKeepsOneNormalRow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.UpdateAIControlSettings(context.Background(), "operate", []string{"custom"}, "test"); err != nil {
		t.Fatal(err)
	}
	tok := mintAgent(t, rt, "w", []string{AbilityRead, AbilityWrite}, agentIdentity{Name: "wbot"})
	stdioReq(rt, http.MethodPost, "/api/v1/apps", tok, map[string]string{mcpToolTagHeader: "create_app", mcpArgsTagHeader: "name"})
	tagRows, all := stdioRows(t, rt, cookie, "wbot")
	if tagRows != 0 || len(all) != 1 || all[0].Method != http.MethodPost || !strings.Contains(string(all[0].Detail), "create_app") {
		t.Fatalf("rows = %+v, tag rows %d", all, tagRows)
	}
}

func TestMCPStdioTagIgnoredWithoutValidToken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tag := map[string]string{mcpToolTagHeader: "list_apps"}
	// Session cookie caller: not a token actor, so no row.
	r := authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", "")
	r.Header.Set(mcpToolTagHeader, "list_apps")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, r)
	if code := stdioReq(rt, http.MethodGet, "/api/v1/apps", "unknown-token", tag); code != http.StatusUnauthorized {
		t.Fatalf("unknown token = %d", code)
	}
	if n, all := stdioRows(t, rt, cookie, ""); n != 0 {
		t.Fatalf("rows = %+v", all)
	}
}

func TestMCPStdioTagSanitizedAndBounded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.UpdateAIControlSettings(context.Background(), "operate", []string{"custom"}, "test"); err != nil {
		t.Fatal(err)
	}
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{Name: "sbot"})
	var names []string
	for i := 0; i < 100; i++ {
		names = append(names, "arg"+strconv.Itoa(i))
	}
	stdioGet(rt, tok, map[string]string{
		mcpToolTagHeader: "list\u202e_apps\"\\" + strings.Repeat("x", 500),
		mcpArgsTagHeader: strings.Join(names, ","),
	})
	_, all := stdioRows(t, rt, cookie, "sbot")
	if len(all) != 1 {
		t.Fatalf("rows = %d", len(all))
	}
	var d struct {
		Tool  string   `json:"tool"`
		Names []string `json:"argument_names"`
	}
	if err := json.Unmarshal(all[0].Detail, &d); err != nil {
		t.Fatalf("detail not valid JSON: %v", err)
	}
	if len(d.Tool) > 64 || strings.ContainsAny(d.Tool, "\u202e\"\\") || len(d.Names) != 32 {
		t.Fatalf("tool %q names %d", d.Tool, len(d.Names))
	}
}

func TestMCPStdioTagRowCap(t *testing.T) {
	t.Setenv(envMCPTagRPM, "2")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.UpdateAIControlSettings(context.Background(), "operate", []string{"custom"}, "test"); err != nil {
		t.Fatal(err)
	}
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{Name: "cbot"})
	for i := 0; i < 5; i++ {
		if code := stdioGet(rt, tok, map[string]string{mcpToolTagHeader: "list_apps"}); code != http.StatusOK {
			t.Fatalf("call %d = %d (cap must not block the request)", i, code)
		}
	}
	if n, _ := stdioRows(t, rt, cookie, "cbot"); n != 2 {
		t.Fatalf("tag rows = %d, want 2", n)
	}
}

func TestMCPHTTPIgnoresClientTagHeader(t *testing.T) {
	mcpEnabled(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tok := mintAgent(t, rt, "r", []string{AbilityRead}, agentIdentity{Name: "rbot"})
	mcpPost(rt, rt.Handler(), tok, mcpCallBody, map[string]string{mcpToolTagHeader: "forged_tool"})
	_, all := stdioRows(t, rt, cookie, "rbot")
	for _, e := range all {
		if strings.Contains(string(e.Detail), "forged_tool") || strings.Contains(string(e.Detail), "client_header") {
			t.Fatalf("client tag leaked into row: %s", e.Detail)
		}
	}
}
