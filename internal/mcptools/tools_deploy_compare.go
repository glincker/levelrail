package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDeployCompareTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "compare_deploys",
		Description: "Diff two of an app's deploy attempts (from/to attempt IDs), or from against live state when to is omitted. Reports image and digest, commit SHA, source, status and commits between them. Env vars, ports, domains and limits are not snapshotted and come back as unsnapshotted. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in compareDeploysInput) (*mcp.CallToolResult, apiclient.DeployCompareResource, error) {
		cmp, err := client.CompareDeploys(ctx, in.Name, in.From, in.To)
		if err != nil {
			return nil, apiclient.DeployCompareResource{}, fmt.Errorf("compare deploys for app %q: %w", in.Name, err)
		}
		return nil, cmp, nil
	})
}

type compareDeploysInput struct {
	Name string `json:"name" jsonschema:"the app's name"`
	From string `json:"from" jsonschema:"deploy attempt id to compare from"`
	To   string `json:"to,omitempty" jsonschema:"deploy attempt id to compare to; default is the app's current live state"`
}
