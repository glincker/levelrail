package mcptools

import (
	"context"
	"encoding/json"

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
			c.InputSchema = slimInputSchema(c.InputSchema)
			trimmed.Tools[i] = &c
		}
		return &trimmed, nil
	}
}

// slimInputSchema drops what a client can infer: additionalProperties:false
// (the server still enforces it on calls) and the null arm of nullable types.
func slimInputSchema(schema any) any {
	b, err := json.Marshal(schema)
	if err != nil {
		return schema
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return schema
	}
	slimSchemaNode(m)
	return m
}

func slimSchemaNode(node any) {
	switch v := node.(type) {
	case map[string]any:
		if ap, ok := v["additionalProperties"].(bool); ok && !ap {
			delete(v, "additionalProperties")
		}
		if types, ok := v["type"].([]any); ok && len(types) == 2 && types[0] == "null" {
			v["type"] = types[1]
		}
		for _, child := range v {
			slimSchemaNode(child)
		}
	case []any:
		for _, child := range v {
			slimSchemaNode(child)
		}
	}
}
