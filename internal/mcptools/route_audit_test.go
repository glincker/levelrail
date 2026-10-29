package mcptools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var routeDeclRe = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) (/[^"]*)",\s*rt\.requireAbility(?:ForResource|Decided)?\((\w+)`)

type apiRoute struct {
	method, pattern, ability string
	re                       *regexp.Regexp
	literals                 int
}

// loadAPIRoutes reads the router's own registrations so the ability each
// route is gated at comes from the source of truth, not a copy.
func loadAPIRoutes(t *testing.T) []apiRoute {
	t.Helper()
	files, err := filepath.Glob("../api/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob api sources: %v", err)
	}
	wild := regexp.MustCompile(`\{[^}]+\}`)
	var routes []apiRoute
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // package-local glob
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range routeDeclRe.FindAllStringSubmatch(string(src), -1) {
			var expr strings.Builder
			pos := 0
			for _, loc := range wild.FindAllStringIndex(m[2], -1) {
				expr.WriteString(regexp.QuoteMeta(m[2][pos:loc[0]]))
				if strings.HasSuffix(m[2][loc[0]:loc[1]], "...}") {
					expr.WriteString(".+")
				} else {
					expr.WriteString("[^/]+")
				}
				pos = loc[1]
			}
			expr.WriteString(regexp.QuoteMeta(m[2][pos:]))
			routes = append(routes, apiRoute{
				method: m[1], pattern: m[2], ability: m[3],
				re:       regexp.MustCompile("^" + expr.String() + "$"),
				literals: len(wild.ReplaceAllString(m[2], "")),
			})
		}
	}
	return routes
}

func matchRoute(routes []apiRoute, method, path string) (apiRoute, bool) {
	var best apiRoute
	found := false
	for _, r := range routes {
		if r.method == method && r.re.MatchString(path) && (!found || r.literals > best.literals) {
			best, found = r, true
		}
	}
	return best, found
}

// placeholder builds a schema-valid value so a tool reaches its API call.
func placeholder(schema map[string]any) any {
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	typ, _ := schema["type"].(string)
	if types, ok := schema["type"].([]any); ok {
		for _, v := range types {
			if name, _ := v.(string); name != "null" {
				typ = name
				break
			}
		}
	}
	switch typ {
	case "integer", "number":
		if floor, ok := schema["minimum"].(float64); ok {
			return floor
		}
		return 1
	case "boolean":
		return true
	case "array":
		if items, ok := schema["items"].(map[string]any); ok {
			return []any{placeholder(items)}
		}
		return []any{}
	case "object":
		return objectArgs(schema)
	}
	return "x"
}

func objectArgs(schema map[string]any) map[string]any {
	out := map[string]any{}
	props, _ := schema["properties"].(map[string]any)
	req, _ := schema["required"].([]any)
	for _, name := range req {
		key, _ := name.(string)
		if p, ok := props[key].(map[string]any); ok {
			out[key] = placeholder(p)
		} else {
			out[key] = "x"
		}
	}
	return out
}

type recordedCall struct{ method, path string }

// auditGaps lists tools whose mutating or sensitive API call is not
// audited today. Each entry needs a reason; do not add one silently.
var auditGaps = map[string]string{}

// unexercised lists tools the placeholder args cannot drive to an API
// call (client side validation, no HTTP at all).
var unexercised = map[string]string{}

func TestMCPToolRoutesAreAudited(t *testing.T) {
	routes := loadAPIRoutes(t)

	var mu sync.Mutex
	var calls []recordedCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, recordedCall{r.Method, r.URL.Path})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	server, _ := mcptools.NewServerWithOptions(apiclient.NewClient(srv.URL, "t"), mcptools.Options{Mode: mcptools.ModeFull, AllowWrite: true})
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	go func() { _ = server.Run(ctx, st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "route-audit", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	var problems []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		meta, ok := mcptools.Lookup(tool.Name)
		if !ok {
			continue
		}
		needsAudit := meta.Class != mcptools.ClassRead || meta.Sensitive()
		if !needsAudit {
			continue
		}
		var args map[string]any
		if raw, err := json.Marshal(tool.InputSchema); err == nil {
			var schema map[string]any
			_ = json.Unmarshal(raw, &schema)
			args = objectArgs(schema)
		}
		mu.Lock()
		calls = nil
		mu.Unlock()
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: args})
		mu.Lock()
		got := append([]recordedCall(nil), calls...)
		mu.Unlock()

		audited := false
		for _, c := range got {
			route, found := matchRoute(routes, c.method, c.path)
			if !found {
				continue
			}
			if api.RouteAudited(c.method, c.path, route.ability) {
				audited = true
			} else if meta.Class != mcptools.ClassRead && c.method != http.MethodGet {
				if _, allowed := auditGaps[tool.Name]; !allowed {
					problems = append(problems, tool.Name+": "+c.method+" "+c.path+" is gated at "+route.ability+" and not audited")
				}
			}
		}
		_, gap := auditGaps[tool.Name]
		_, skip := unexercised[tool.Name]
		switch {
		case audited || gap || skip:
		case len(got) == 0:
			problems = append(problems, tool.Name+": made no API call with placeholder args; add it to unexercised with a reason")
		default:
			problems = append(problems, tool.Name+": no audited route among "+describe(got))
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

func describe(calls []recordedCall) string {
	parts := make([]string, 0, len(calls))
	for _, c := range calls {
		parts = append(parts, c.method+" "+c.path)
	}
	return strings.Join(parts, ", ")
}
