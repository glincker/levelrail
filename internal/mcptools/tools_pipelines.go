package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const explainLogTail = 60

type listPipelineRunsInput struct {
	Name     string `json:"name" jsonschema:"the app's name"`
	Pipeline string `json:"pipeline,omitempty" jsonschema:"only runs of this pipeline name"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum runs to return, newest first (default 20)"`
}

type pipelineRunInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	RunID string `json:"run_id" jsonschema:"the pipeline run id"`
}

type pipelineRunsOutput struct {
	Runs []apiclient.PipelineRunResource `json:"runs"`
}

// pipelineRunExplanation is the facts an agent needs to reason about a run;
// it carries no verdict beyond what the run itself recorded.
type pipelineRunExplanation struct {
	Run               apiclient.PipelineRunResource `json:"run"`
	Summary           string                        `json:"summary"`
	FailedJob         string                        `json:"failed_job,omitempty"`
	FailedStep        string                        `json:"failed_step,omitempty"`
	FailedStepReason  string                        `json:"failed_step_reason,omitempty"`
	ExitCode          *int                          `json:"exit_code,omitempty"`
	LogTail           []string                      `json:"log_tail,omitempty"`
	WaitingOnApproval []apiclient.PipelineApproval  `json:"waiting_on_approval,omitempty"`
	SkippedJobs       []string                      `json:"skipped_jobs,omitempty"`
}

func registerPipelineTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_pipeline_runs",
		Description: "List an app's CI/CD pipeline runs, newest first, with status, trigger, ref, and the reason string for each. Filter by pipeline name. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listPipelineRunsInput) (*mcp.CallToolResult, pipelineRunsOutput, error) {
		runs, err := client.ListPipelineRuns(ctx, in.Name, in.Pipeline, in.Limit)
		if err != nil {
			return nil, pipelineRunsOutput{}, fmt.Errorf("list pipeline runs for app %q: %w", in.Name, err)
		}
		if runs == nil {
			runs = []apiclient.PipelineRunResource{}
		}
		return nil, pipelineRunsOutput{Runs: runs}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "explain_pipeline_run",
		Description: "Explain a pipeline run: its status and reason, the first failed job and step with exit code and the last lines of that step's output, any approval gates it is waiting on, and jobs that were skipped. Read-only; it reports what happened and never approves, cancels, or re-runs anything.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pipelineRunInput) (*mcp.CallToolResult, pipelineRunExplanation, error) {
		run, err := client.GetPipelineRun(ctx, in.Name, in.RunID)
		if err != nil {
			return nil, pipelineRunExplanation{}, fmt.Errorf("get pipeline run %q: %w", in.RunID, err)
		}
		return nil, explainPipelineRun(ctx, client, in.Name, run), nil
	})
}

func explainPipelineRun(ctx context.Context, client *apiclient.Client, app string, run apiclient.PipelineRunResource) pipelineRunExplanation {
	ex := pipelineRunExplanation{Run: run}
	for _, a := range run.Approvals {
		if a.Decision == "" {
			ex.WaitingOnApproval = append(ex.WaitingOnApproval, a)
		}
	}
	for _, j := range run.Jobs {
		if j.Status == "skipped" {
			ex.SkippedJobs = append(ex.SkippedJobs, j.Key+": "+j.Reason)
		}
		if ex.FailedJob != "" || j.Status != "failed" {
			continue
		}
		ex.FailedJob = j.Key
		for _, s := range j.Steps {
			if s.Status == "failed" {
				ex.FailedStep, ex.FailedStepReason, ex.ExitCode = s.Name, s.Reason, s.ExitCode
				ex.LogTail = stepLogTail(ctx, client, app, run.ID, j.Key, s.Index)
				break
			}
		}
		if ex.FailedStep == "" {
			ex.FailedStepReason = j.Reason
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Run #%d of %s is %s", run.Number, run.PipelineName, run.Status)
	if run.Reason != "" {
		fmt.Fprintf(&sb, " (%s)", run.Reason)
	}
	sb.WriteString(".")
	if ex.FailedJob != "" {
		fmt.Fprintf(&sb, " Job %q failed", ex.FailedJob)
		if ex.FailedStep != "" {
			fmt.Fprintf(&sb, " at step %q: %s", ex.FailedStep, ex.FailedStepReason)
		}
		sb.WriteString(".")
	}
	if len(ex.WaitingOnApproval) > 0 {
		fmt.Fprintf(&sb, " Waiting on %d approval gate(s).", len(ex.WaitingOnApproval))
	}
	ex.Summary = sb.String()
	return ex
}

func stepLogTail(ctx context.Context, client *apiclient.Client, app, runID, job string, step int) []string {
	lines, err := client.ListPipelineRunLogs(ctx, app, runID, job, 0, 5000)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range lines {
		if l.Step == step {
			out = append(out, l.Line)
		}
	}
	if len(out) > explainLogTail {
		out = out[len(out)-explainLogTail:]
	}
	return out
}
