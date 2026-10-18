package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type resilienceInput struct{}

type resilienceOutput struct {
	Results []string `json:"results" jsonschema:"one line per scenario"`
}

func registerResilienceTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_resilience_report",
		Description: "Latest chaos scenario results.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ resilienceInput) (*mcp.CallToolResult, resilienceOutput, error) {
		rep, err := client.GetResilienceReport(ctx)
		if err != nil {
			return nil, resilienceOutput{}, fmt.Errorf("get resilience report: %w", err)
		}
		out := resilienceOutput{Results: []string{}}
		for _, r := range rep.Latest {
			out.Results = append(out.Results, fmt.Sprintf("%s %s: recovery %.0f ms, %d of %d requests failed, %s", r.Scenario, r.Status, r.RecoveryMS, r.RequestErrors, r.Requests, r.RanAt.Format("2006-01-02")))
		}
		for _, n := range rep.Unmeasured {
			out.Results = append(out.Results, n+" not yet measured")
		}
		return nil, out, nil
	})
}
