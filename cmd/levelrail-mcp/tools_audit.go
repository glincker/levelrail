package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAuditTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_audit_log",
		Description: "List recorded write/deploy/root-tier requests across the control plane, newest first: who did what, when, from where, and whether it succeeded. Useful for answering 'who changed this env var and broke prod' or auditing recent admin activity. Read-only; does not modify anything.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in auditLogInput) (*mcp.CallToolResult, []apiclient.AuditLogEntryResource, error) {
		entries, err := client.ListAuditLog(ctx, apiclient.ListAuditLogOptions{
			Limit:  in.Limit,
			Before: in.Before,
			Path:   in.Path,
			Method: in.Method,
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
}
