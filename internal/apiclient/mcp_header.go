package apiclient

import (
	"context"
	"strings"
)

const (
	// MCPToolHeader names the MCP tool behind a request. It is advisory: the
	// control plane only records it, never trusts it for access decisions.
	MCPToolHeader = "X-Levelrail-Mcp-Tool"
	// MCPArgsHeader lists the argument names (never values) of that tool call.
	MCPArgsHeader = "X-Levelrail-Mcp-Args"
)

type mcpCall struct {
	tool string
	args string
}

type mcpCallKey struct{}

// WithMCPCall returns a context whose requests carry the tool name and
// argument names in the MCP headers.
func WithMCPCall(ctx context.Context, tool string, argNames []string) context.Context {
	if tool == "" {
		return ctx
	}
	return context.WithValue(ctx, mcpCallKey{}, mcpCall{tool: tool, args: strings.Join(argNames, ",")})
}

func mcpCallFrom(ctx context.Context) mcpCall {
	c, _ := ctx.Value(mcpCallKey{}).(mcpCall)
	return c
}
