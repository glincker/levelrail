package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLogSignaturesTool(t *testing.T) {
	var gotPath, gotBaseline string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotBaseline = r.URL.Path, r.URL.Query().Get("baseline")
		_ = json.NewEncoder(w).Encode(map[string]any{"scanned": 9, "total": 1, "signatures": []map[string]any{
			{"id": "a1", "template": "db timeout after <num>ms", "count": 4, "last_seen": time.Now().UTC(), "new": true, "sample": "ERROR db timeout"},
		}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "log_signatures", Arguments: map[string]any{"app": "web", "since": "6h", "baseline": true}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %s", err, toolResultText(res))
	}
	var out logSignaturesOutput
	decodeStructured(t, res, &out)
	if len(out.Signatures) != 1 || out.Signatures[0].Count != 4 || !out.Signatures[0].New {
		t.Fatalf("output = %+v", out)
	}
	if gotPath != "/api/v1/apps/web/logs/signatures" || gotBaseline != "true" {
		t.Fatalf("request path=%q baseline=%q", gotPath, gotBaseline)
	}
}
