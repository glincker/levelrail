package mcptools

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	auditMaxArgKeys = 32
	auditMaxKeyLen  = 64
)

// AuditCall describes one tools/call. It carries argument names only,
// never values, because tool inputs can carry secrets.
type AuditCall struct {
	Tool          string
	ArgumentNames []string
	Outcome       string // "ok" or "error"
	Duration      time.Duration
}

// AuditSink receives one AuditCall per tools/call, including calls the
// server rejects before a handler runs.
type AuditSink func(ctx context.Context, call AuditCall)

// auditMiddleware sits outermost so denied, unknown and failed calls are
// recorded the same way as successful ones, whichever transport serves them.
func auditMiddleware(sink AuditSink) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			start := time.Now()
			res, err := next(ctx, method, req)
			outcome := "ok"
			if cr, isCall := res.(*mcp.CallToolResult); err != nil || (isCall && cr.IsError) {
				outcome = "error"
			}
			tool, keys := callSummary(req)
			sink(ctx, AuditCall{Tool: tool, ArgumentNames: keys, Outcome: outcome, Duration: time.Since(start)})
			return res, err
		}
	}
}

// tagAPICallsMiddleware puts the tool name and argument names on the context
// so apiclient forwards them as headers on each API request the tool makes.
func tagAPICallsMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				tool, keys := callSummary(req)
				ctx = apiclient.WithMCPCall(ctx, tool, keys)
			}
			return next(ctx, method, req)
		}
	}
}

func callSummary(req mcp.Request) (string, []string) {
	var name string
	var raw json.RawMessage
	switch p := req.GetParams().(type) {
	case *mcp.CallToolParamsRaw:
		name, raw = p.Name, p.Arguments
	case *mcp.CallToolParams:
		name = p.Name
		if p.Arguments != nil {
			raw, _ = json.Marshal(p.Arguments)
		}
	}
	var args map[string]json.RawMessage
	_ = json.Unmarshal(raw, &args)
	keys := make([]string, 0, len(args))
	for k := range args {
		if len(keys) < auditMaxArgKeys {
			keys = append(keys, truncateRunes(k, auditMaxKeyLen))
		}
	}
	slices.Sort(keys)
	return truncateRunes(name, auditMaxKeyLen), keys
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
