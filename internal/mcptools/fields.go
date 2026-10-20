package mcptools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// projectableTools are the heaviest list tools; each accepts fields= to keep only the named keys of every item.
var projectableTools = map[string]struct{}{
	"list_apps":            {},
	"list_deployments":     {},
	"list_deploy_attempts": {},
	"list_audit_log":       {},
}

// projectFields keeps only the named keys of every object in each top-level array of raw (or of raw itself when it is an array). Scalars such as next_cursor survive.
func projectFields(raw []byte, fields []string) ([]byte, bool) {
	keep := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		keep[f] = struct{}{}
	}
	var top any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, false
	}
	switch v := top.(type) {
	case []any:
		top = projectItems(v, keep)
	case map[string]any:
		for k, el := range v {
			if arr, ok := el.([]any); ok {
				v[k] = projectItems(arr, keep)
			}
		}
	default:
		return nil, false
	}
	out, err := json.Marshal(top)
	if err != nil {
		return nil, false
	}
	return out, true
}

func projectItems(items []any, keep map[string]struct{}) []any {
	for i, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		slim := make(map[string]any, len(keep))
		for k := range keep {
			if val, present := obj[k]; present {
				slim[k] = val
			}
		}
		items[i] = slim
	}
	return items
}

func splitFields(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// fieldsProjection rewrites the result of a projectable tool call that carried fields=.
func fieldsProjection(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		if method != "tools/call" || err != nil {
			return res, err
		}
		call, ok := req.(*mcp.CallToolRequest)
		out, isResult := res.(*mcp.CallToolResult)
		if !ok || !isResult || call.Params == nil || out.IsError {
			return res, err
		}
		if _, listed := projectableTools[call.Params.Name]; !listed {
			return res, err
		}
		var args struct {
			Fields string `json:"fields"`
		}
		if json.Unmarshal(call.Params.Arguments, &args) != nil {
			return res, err
		}
		fields := splitFields(args.Fields)
		if len(fields) == 0 {
			return res, err
		}
		structured, ok := out.StructuredContent.(json.RawMessage)
		if !ok {
			return res, err
		}
		slim, ok := projectFields(structured, fields)
		if !ok {
			return res, err
		}
		projected := *out
		projected.StructuredContent = json.RawMessage(slim)
		projected.Content = []mcp.Content{&mcp.TextContent{Text: string(slim)}}
		return &projected, nil
	}
}
