package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPipelineTools(t *testing.T) {
	two := 2
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/web/pipeline-runs":
			_ = json.NewEncoder(w).Encode([]apiclient.PipelineRunResource{{ID: "r1", Number: 3, PipelineName: "ci", Status: "failed"}})
		case "/api/v1/apps/web/pipeline-runs/r1":
			_ = json.NewEncoder(w).Encode(apiclient.PipelineRunResource{
				ID: "r1", Number: 3, PipelineName: "ci", Status: "failed", Reason: "job test failed",
				Jobs: []apiclient.PipelineJobView{
					{Key: "test", Status: "failed", Steps: []apiclient.PipelineStepView{{Index: 1, Name: "go test", Status: "failed", Reason: "exited with code 2", ExitCode: &two}}},
					{Key: "deploy", Status: "skipped", Reason: "a needed job did not succeed"},
				},
				Approvals: []apiclient.PipelineApproval{{ID: 9, Job: "deploy"}},
			})
		case "/api/v1/apps/web/pipeline-runs/r1/logs":
			_ = json.NewEncoder(w).Encode([]apiclient.PipelineLogLine{
				{ID: 1, Job: "test", Step: 0, Line: "checkout"},
				{ID: 2, Job: "test", Step: 1, Line: "FAIL TestX"},
			})
		default:
			t.Errorf("unexpected %s %q", r.Method, r.URL.Path)
		}
	})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_pipeline_runs", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("list_pipeline_runs: %v", err)
	}
	var list pipelineRunsOutput
	decodeStructured(t, res, &list)
	if len(list.Runs) != 1 || list.Runs[0].Status != "failed" {
		t.Fatalf("runs = %+v", list.Runs)
	}

	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "explain_pipeline_run", Arguments: map[string]any{"name": "web", "run_id": "r1"}})
	if err != nil {
		t.Fatalf("explain_pipeline_run: %v", err)
	}
	var ex pipelineRunExplanation
	decodeStructured(t, res, &ex)
	if ex.FailedJob != "test" || ex.FailedStep != "go test" || ex.ExitCode == nil || *ex.ExitCode != 2 {
		t.Fatalf("explanation = %+v", ex)
	}
	if len(ex.LogTail) != 1 || ex.LogTail[0] != "FAIL TestX" {
		t.Errorf("log tail = %v, want only the failed step's lines", ex.LogTail)
	}
	if len(ex.WaitingOnApproval) != 1 || len(ex.SkippedJobs) != 1 || !strings.Contains(ex.Summary, "failed at step") {
		t.Errorf("explanation = %+v", ex)
	}
}
