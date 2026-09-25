package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/untrusted"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// wrapUntrustedResult sanitizes a handler's output and puts the text form
// in a delimited untrusted block. The structured copy is sanitized too but
// carries no delimiter, so a client should prefer the text content.
func wrapUntrustedResult[In, Out any](name string, h mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		limits := untrusted.LimitsFromEnv()
		res, out, err := h(ctx, req, in)
		if err != nil {
			var zero Out
			return nil, zero, errors.New(untrusted.Sanitize(err.Error(), limits.Field))
		}
		clean, err := untrusted.SanitizeValue(out, limits)
		if err != nil {
			var zero Out
			return nil, zero, fmt.Errorf("sanitize %s output: %w", name, err)
		}
		text, err := json.Marshal(clean)
		if err != nil {
			var zero Out
			return nil, zero, fmt.Errorf("marshal %s output: %w", name, err)
		}
		if res == nil {
			res = &mcp.CallToolResult{}
		}
		res.Content = []mcp.Content{&mcp.TextContent{Text: untrusted.Wrap(name, string(text), limits)}}
		return res, clean, nil
	}
}
