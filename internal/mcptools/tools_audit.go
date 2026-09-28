package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAuditTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_audit_log",
		Description: "List recorded write/deploy/root-tier requests across the control plane, newest first: who did what, when, from where, and whether it succeeded. Useful for answering 'who changed this env var and broke prod' or auditing recent admin activity. Entries made by an agent token carry agent_name and the MCP client name and version. Filter with q (text search), agent and status=failed. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in auditLogInput) (*mcp.CallToolResult, []apiclient.AuditLogEntryResource, error) {
		entries, err := client.ListAuditLog(ctx, apiclient.ListAuditLogOptions{
			Limit:      in.Limit,
			Before:     in.Before,
			Path:       in.Path,
			Method:     in.Method,
			Search:     in.Query,
			FailedOnly: in.Status == "failed",
			Agent:      in.Agent,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list audit log: %w", err)
		}
		return nil, entries, nil
	})
}

type auditLogInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"max entries to return, default the server's own default"`
	Before string `json:"before,omitempty" jsonschema:"only return entries recorded before this RFC3339 timestamp, for paging backward"`
	Path   string `json:"path,omitempty" jsonschema:"only return entries whose request path matches this"`
	Method string `json:"method,omitempty" jsonschema:"only return entries whose HTTP method matches this, e.g. POST"`
	Query  string `json:"q,omitempty" jsonschema:"case-insensitive substring match across actor, ability, method, path and remote address"`
	Status string `json:"status,omitempty" jsonschema:"set to failed to only return entries with an HTTP status of 400 or above"`
	Agent  string `json:"agent,omitempty" jsonschema:"only return entries made with a token labeled with this agent name"`
}
