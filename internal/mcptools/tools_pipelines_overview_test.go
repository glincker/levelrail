package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListAllPipelineRunsTool(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pipeline-runs" {
			t.Errorf("unexpected %s %q", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.Query().Encode()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.PipelineRunRowsPage{
			Runs:       []apiclient.PipelineRunRow{{ID: "r1", App: "web", Pipeline: "ci", Status: "failed"}},
			NextCursor: "abc",
		})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_all_pipeline_runs", Arguments: map[string]any{"status": "failed", "limit": 5}})
	if err != nil {
		t.Fatalf("list_all_pipeline_runs: %v", err)
	}
	var out allPipelineRunsOutput
	decodeStructured(t, res, &out)
	if len(out.Runs) != 1 || out.Runs[0].App != "web" || out.NextCursor != "abc" {
		t.Fatalf("output = %+v", out)
	}
	if gotQuery != "limit=5&status=failed" {
		t.Errorf("query = %q", gotQuery)
	}
}
