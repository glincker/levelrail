package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func tagSession(t *testing.T, tag bool, seen *http.Header) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.AuditLogEntryResource{})
	}))
	t.Cleanup(srv.Close)
	server, _ := NewServerWithOptions(apiclient.NewClient(srv.URL, "test-token"), Options{TagAPICalls: tag})
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestTagAPICallsSendsToolAndArgumentNamesOnly(t *testing.T) {
	var seen http.Header
	s := tagSession(t, true, &seen)
	if _, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_audit_log", Arguments: map[string]any{"agent": "secret-agent-value"}}); err != nil {
		t.Fatal(err)
	}
	if got := seen.Get(apiclient.MCPToolHeader); got != "list_audit_log" {
		t.Errorf("tool header = %q", got)
	}
	if got := seen.Get(apiclient.MCPArgsHeader); got != "agent" {
		t.Errorf("args header = %q, want names only", got)
	}
}

func TestTagAPICallsOffByDefault(t *testing.T) {
	var seen http.Header
	s := tagSession(t, false, &seen)
	if _, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_audit_log", Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if seen.Get(apiclient.MCPToolHeader) != "" {
		t.Error("tool header sent without TagAPICalls")
	}
}
