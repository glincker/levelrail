// Command mcp-eval runs ten scripted agent tasks through the real MCP server
// and reports pass or fail and the estimated tokens each one costs.
//
// By default it serves canned responses from an in-process demo API, so it
// needs no network, no model and no running instance. With -api and -token
// it drives a real control plane instead and only checks that every call
// succeeds. Usage: go run ./scripts/mcp-eval [-api URL -token TOKEN] [-json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const charsPerToken = 4

type call struct {
	tool string
	args map[string]any
	want string
}

type task struct {
	name  string
	calls []call
}

var tasks = []task{
	{"identify the caller", []call{{"whoami", nil, `"name"`}}},
	{"list apps", []call{{"list_apps", nil, `"web"`}}},
	{"read app status", []call{{"get_app_status", map[string]any{"name": "web"}, "Ready"}}},
	{"list deploy conditions", []call{{"list_deploys", map[string]any{"name": "web"}, "Ready"}}},
	{"discover toolsets", []call{{"list_toolsets", nil, "inbox"}}},
	{"read the inbox", []call{
		{"enable_toolset", map[string]any{"toolsets": []string{"inbox"}}, "inbox"},
		{"list_inbox", map[string]any{"unread_only": true}, `"unread"`},
	}},
	{"simulate placement", []call{
		{"enable_toolset", map[string]any{"toolsets": []string{"nodes"}}, "nodes"},
		{"simulate_placement", map[string]any{"cpu_milli": 500, "memory_mib": 256, "count": 2}, `"placed"`},
	}},
	{"simulate a zone failure", []call{
		{"enable_toolset", map[string]any{"toolsets": []string{"nodes"}}, "nodes"},
		{"simulate_placement", map[string]any{"zone": "eu-1"}, `"displaced"`},
	}},
	{"verify the audit chain", []call{
		{"enable_toolset", map[string]any{"toolsets": []string{"audit"}}, "audit"},
		{"verify_audit_log", nil, `"ok":true`},
	}},
	{"read the edge overview", []call{
		{"enable_toolset", map[string]any{"toolsets": []string{"metrics"}}, "metrics"},
		{"get_cdn_overview", map[string]any{"window": "1h"}, `"window"`},
	}},
}

var demoRoutes = map[string]string{
	"/api/v1/auth/whoami":                `{"kind":"token","name":"demo","abilities":["read","root"]}`,
	"/api/v1/apps":                       `[{"name":"web","image":"nginx:1","port":80,"replicas":1,"strategy":"rolling"}]`,
	"/api/v1/apps/web/deploys":           `[{"type":"Ready","status":"True","reason":"Converged","message":"1/1 replicas ready"}]`,
	"/api/v1/inbox":                      `[{"id":"i1","read":false,"severity":"warning","event":"crashloop","app":"web"}]`,
	"/api/v1/inbox/unread-count":         `{"unread":1}`,
	"/api/v1/nodes/placement-simulation": `{"request":{"count":2},"placed":[{"node_id":"n1"},{"node_id":"n2"}],"unplaced":0,"zones":[]}`,
	"/api/v1/nodes/failure-simulation":   `{"zone":"eu-1","lost_node_ids":["n1"],"displaced":2,"placed":[],"unplaced":0,"zones":[]}`,
	"/api/v1/audit-log/verify":           `{"ok":true,"checked":120,"problems":[]}`,
	"/api/v1/cdn/overview":               `{"window":"1h","has_traffic":true,"requests":1200,"nodes":[]}`,
}

type result struct {
	Task   string `json:"task"`
	Pass   bool   `json:"pass"`
	Tokens int    `json:"tokens"`
	Detail string `json:"detail,omitempty"`
}

func main() {
	api := flag.String("api", "", "base URL of a real control plane; default is the in-process demo API")
	token := flag.String("token", "", "API token for -api")
	asJSON := flag.Bool("json", false, "print results as JSON")
	flag.Parse()

	base, tok, live := *api, *token, *api != ""
	if !live {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, ok := demoRoutes[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()
		base, tok = srv.URL, "demo-token"
	}

	var results []result
	failed := 0
	for _, t := range tasks {
		r := run(context.Background(), base, tok, live, t)
		if !r.Pass {
			failed++
		}
		results = append(results, r)
	}
	report(results, *asJSON)
	if failed > 0 {
		os.Exit(1)
	}
}

func run(ctx context.Context, base, tok string, live bool, t task) result {
	res := result{Task: t.name}
	server, _ := mcptools.NewServerWithOptions(apiclient.NewClient(base, tok), mcptools.Options{Mode: mcptools.ModeFull, Dynamic: true})
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "mcp-eval", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		res.Detail = "connect: " + err.Error()
		return res
	}
	defer func() { _ = session.Close() }()

	tokens := 0
	listing := func() error {
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(tools.Tools)
		tokens += len(b) / charsPerToken
		return nil
	}
	if err := listing(); err != nil {
		res.Detail = "list tools: " + err.Error()
		return res
	}
	for _, c := range t.calls {
		args, _ := json.Marshal(c.args)
		out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: c.tool, Arguments: c.args})
		if err != nil {
			res.Detail = fmt.Sprintf("%s: %v", c.tool, err)
			res.Tokens = tokens
			return res
		}
		text := resultText(out)
		tokens += (len(c.tool) + len(args) + len(text)) / charsPerToken
		if out.IsError {
			res.Detail = fmt.Sprintf("%s: %s", c.tool, truncate(text, 120))
			res.Tokens = tokens
			return res
		}
		if !live && !strings.Contains(text, c.want) {
			res.Detail = fmt.Sprintf("%s: result lacks %s", c.tool, c.want)
			res.Tokens = tokens
			return res
		}
		if c.tool == "enable_toolset" {
			if err := listing(); err != nil {
				res.Detail = "list tools: " + err.Error()
				res.Tokens = tokens
				return res
			}
		}
	}
	res.Pass, res.Tokens = true, tokens
	return res
}

func resultText(r *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func report(results []result, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TASK\tRESULT\tTOKENS\tDETAIL")
	total, passed := 0, 0
	for _, r := range results {
		status := "FAIL"
		if r.Pass {
			status, passed = "pass", passed+1
		}
		total += r.Tokens
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.Task, status, r.Tokens, r.Detail)
	}
	_ = w.Flush()
	fmt.Printf("\n%d/%d passed, ~%d tokens in total\n", passed, len(results), total)
}
