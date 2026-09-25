package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listAllPipelineRunsInput struct {
	Status   string `json:"status,omitempty" jsonschema:"running, failed, succeeded, cancelled, waiting_approval, or held"`
	App      string `json:"app,omitempty" jsonschema:"only runs of this app"`
	Pipeline string `json:"pipeline,omitempty" jsonschema:"only runs of this pipeline name"`
	Trigger  string `json:"trigger,omitempty" jsonschema:"push, pull_request, tag, manual, schedule, or api"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum runs to return, newest first (default 50)"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous call"`
}

type allPipelineRunsOutput struct {
	Runs       []apiclient.PipelineRunRow `json:"runs"`
	NextCursor string                     `json:"next_cursor,omitempty"`
}

func registerPipelineOverviewTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_all_pipeline_runs",
		Description: "List recent CI/CD pipeline runs across every app the caller can read, newest first, with app, pipeline, status, trigger, ref, short sha, duration, and whether a run is waiting on an approval or hold. Filter by status, app, pipeline, or trigger. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listAllPipelineRunsInput) (*mcp.CallToolResult, allPipelineRunsOutput, error) {
		page, err := client.ListAllPipelineRuns(ctx, apiclient.PipelineRunsQuery{
			Status: in.Status, App: in.App, Pipeline: in.Pipeline, Trigger: in.Trigger, Cursor: in.Cursor, Limit: in.Limit,
		})
		if err != nil {
			return nil, allPipelineRunsOutput{}, fmt.Errorf("list pipeline runs across apps: %w", err)
		}
		if page.Runs == nil {
			page.Runs = []apiclient.PipelineRunRow{}
		}
		return nil, allPipelineRunsOutput{Runs: page.Runs, NextCursor: page.NextCursor}, nil
	})
}
