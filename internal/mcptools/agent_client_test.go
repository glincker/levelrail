package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestToolCallsReportMCPClientIdentity(t *testing.T) {
	var gotHeader, gotAgent string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get(apiclient.AgentClientHeader)
		gotAgent = r.URL.Query().Get("agent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.AuditLogEntryResource{{ID: "a1", AgentName: "deploy-bot", AgentClient: "test-client/0.0.0"}})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_audit_log", Arguments: map[string]any{"agent": "deploy-bot"}})
	if err != nil {
		t.Fatal(err)
	}
	if gotHeader != "test-client/0.0.0" {
		t.Errorf("%s = %q, want the MCP client name and version", apiclient.AgentClientHeader, gotHeader)
	}
	if gotAgent != "deploy-bot" {
		t.Errorf("agent filter = %q", gotAgent)
	}
	var got []apiclient.AuditLogEntryResource
	decodeStructured(t, result, &got)
	if len(got) != 1 || got[0].AgentName != "deploy-bot" {
		t.Errorf("got %+v", got)
	}
}
