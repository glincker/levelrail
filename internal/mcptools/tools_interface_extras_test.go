package mcptools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInterfaceExtraTools(t *testing.T) {
	var paths []string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/inbox":
			_, _ = w.Write([]byte(`[{"id":"a1","read":false}]`))
		case "/api/v1/inbox/unread-count":
			_, _ = w.Write([]byte(`{"unread":3}`))
		case "/api/v1/nodes/failure-simulation":
			_, _ = w.Write([]byte(`{"zone":"eu","lost_node_ids":["n1"],"displaced":2,"placed":[],"unplaced":1,"zones":[]}`))
		case "/api/v1/nodes/placement-simulation":
			_, _ = w.Write([]byte(`{"request":{"count":2},"placed":[],"unplaced":0,"zones":[]}`))
		case "/api/v1/audit-log/verify":
			_, _ = w.Write([]byte(`{"ok":true,"checked":10,"problems":[]}`))
		case "/api/v1/cdn/overview":
			_, _ = w.Write([]byte(`{"window":"1h","has_traffic":true,"nodes":[]}`))
		default:
			http.NotFound(w, r)
		}
	})
	call := func(name string, args map[string]any) string {
		t.Helper()
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("%s: err=%v result=%s", name, err, toolResultText(res))
		}
		return toolResultText(res)
	}
	if out := call("list_inbox", map[string]any{"unread_only": true}); !strings.Contains(out, `"unread":3`) {
		t.Errorf("list_inbox = %s", out)
	}
	if out := call("simulate_placement", map[string]any{"zone": "eu"}); !strings.Contains(out, `"displaced":2`) {
		t.Errorf("failure simulation = %s", out)
	}
	if out := call("simulate_placement", map[string]any{"count": 2}); !strings.Contains(out, `"unplaced":0`) {
		t.Errorf("placement simulation = %s", out)
	}
	if out := call("verify_audit_log", nil); !strings.Contains(out, `"ok":true`) {
		t.Errorf("verify_audit_log = %s", out)
	}
	if out := call("get_cdn_overview", map[string]any{"window": "1h"}); !strings.Contains(out, `"window":"1h"`) {
		t.Errorf("get_cdn_overview = %s", out)
	}
}
