package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// omitOutputSchemas drops outputSchema from tools/list results. Results
// are still returned as text and structured content; only the schema
// advertisement, most of a tool's listing cost, is left out.
func omitOutputSchemas(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		list, ok := res.(*mcp.ListToolsResult)
		if err != nil || !ok {
			return res, err
		}
		trimmed := *list
		trimmed.Tools = make([]*mcp.Tool, len(list.Tools))
		for i, t := range list.Tools {
			c := *t
			c.OutputSchema = nil
			trimmed.Tools[i] = &c
		}
		return &trimmed, nil
	}
}
