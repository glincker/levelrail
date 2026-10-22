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
		Description: "List recorded write, deploy and root requests, newest first: who, what, when, from where, success. Agent token entries carry agent_name and MCP client. Filter with q, agent, status=failed.",
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
	Limit  int    `json:"limit,omitempty" jsonschema:"max entries; server default"`
	Before string `json:"before,omitempty" jsonschema:"entries before this RFC3339 time"`
	Path   string `json:"path,omitempty" jsonschema:"only this request path"`
	Method string `json:"method,omitempty" jsonschema:"only this HTTP method, e.g. POST"`
	Query  string `json:"q,omitempty" jsonschema:"case-insensitive substring of actor, ability, method, path or address"`
	Status string `json:"status,omitempty" jsonschema:"failed: only status 400 and up"`
	Agent  string `json:"agent,omitempty" jsonschema:"only tokens labeled with this agent name"`
	Fields string `json:"fields,omitempty" jsonschema:"keys to keep per item"`
}
