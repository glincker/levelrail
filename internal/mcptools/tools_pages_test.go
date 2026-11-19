package mcptools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPagesToolsRegisteredWithClasses(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeFull, AllowWrite: true})
	seen := map[string]*mcp.Tool{}
	for _, tool := range tools {
		seen[tool.Name] = tool
	}
	if seen["pages"] == nil || seen["pages_manage"] == nil {
		t.Fatalf("pages tools missing")
	}
	if !seen["pages"].Annotations.ReadOnlyHint {
		t.Errorf("pages must be read only")
	}
	if seen["pages_manage"].Annotations.ReadOnlyHint {
		t.Errorf("pages_manage must not be read only")
	}
}

func TestPagesArgumentValidation(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"pages", map[string]any{"action": "bogus"}, "unknown action"},
		{"pages", map[string]any{"action": "show"}, "name is required"},
		{"pages", map[string]any{"action": "traffic", "name": "a", "window": "30d"}, "24h or 7d"},
		{"pages_manage", map[string]any{"action": "delete", "name": "a"}, "confirm must equal"},
		{"pages_manage", map[string]any{"action": "rollback", "name": "a", "confirm": "b"}, "confirm must equal"},
		{"pages_manage", map[string]any{"action": "deploy", "name": "a"}, "exactly one of dir or files"},
		{"pages_manage", map[string]any{"action": "deploy", "name": "a", "files": map[string]any{"../x": "y"}}, "inside the site"},
	}
	for _, tc := range cases {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || !strings.Contains(toolResultText(res), tc.want) {
			t.Errorf("%s %v: got %q, want %q", tc.tool, tc.args, toolResultText(res), tc.want)
		}
	}
}

func TestPagesDeployInlineFiles(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var uploaded int
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/static-sites/demo" && len(calls) == 1:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"static site not found"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/static-sites":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"demo","build_type":"static","domains":[]}`))
		case r.URL.Path == "/api/v1/apps/demo/uploads":
			b, _ := io.ReadAll(r.Body)
			uploaded = len(b)
			_, _ = w.Write([]byte(`{"hash":"` + strings.Repeat("a", 64) + `","size":10}`))
		case r.URL.Path == "/api/v1/apps/demo/builds":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["upload_hash"] == nil {
				t.Errorf("build body lacks upload_hash: %v", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"dep_1"}`))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1"):
			_, _ = w.Write([]byte(`{"id":"dep_1","outcome":"healthy","status":"succeeded"}`))
		default:
			_, _ = w.Write([]byte(`{"name":"demo","build_type":"static","domains":["demo.example.com"],"published":true}`))
		}
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pages_manage", Arguments: map[string]any{
		"action": "deploy", "name": "demo", "files": map[string]any{"index.html": "<h1>hi</h1>", "css/a.css": "a{}"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolResultText(res))
	}
	var got pagesOutput
	decodeStructured(t, res, &got)
	if got.Status != "healthy" || got.URL != "https://demo.example.com" || got.Files != 2 || uploaded == 0 {
		t.Fatalf("got %+v uploaded=%d calls=%v", got, uploaded, calls)
	}
}

func TestPagesListAndShow(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/static-sites" {
			_, _ = w.Write([]byte(`[{"name":"demo","domains":["d.example.com"]}]`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"demo","build_type":"static","domains":[],"published":true,"file_count":3}`))
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pages", Arguments: map[string]any{"action": "list"}})
	if err != nil {
		t.Fatal(err)
	}
	var got pagesOutput
	decodeStructured(t, res, &got)
	if rows, _ := got.Data.([]any); len(rows) != 1 {
		t.Fatalf("got %+v", got)
	}
}
