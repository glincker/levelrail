package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type failedDeploysInput struct {
	Since string `json:"since,omitempty" jsonschema:"only deploys newer than this Go duration such as 24h; empty uses the server default"`
}

type failedDeploysOutput struct {
	Deploys []apiclient.FailedDeployResource `json:"deploys"`
}

func registerFailedDeployTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_failed_deploys",
		Description: "List each app's latest failed deploy attempt with its error and last_good_image, to pick a rollback target. Filter with since (e.g. 24h).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in failedDeploysInput) (*mcp.CallToolResult, failedDeploysOutput, error) {
		deploys, err := client.ListFailedDeploys(ctx, in.Since)
		if err != nil {
			return nil, failedDeploysOutput{}, fmt.Errorf("list failed deploys: %w", err)
		}
		if deploys == nil {
			deploys = []apiclient.FailedDeployResource{}
		}
		return nil, failedDeploysOutput{Deploys: deploys}, nil
	})
}
