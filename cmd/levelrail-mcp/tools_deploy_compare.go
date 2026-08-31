package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDeployCompareTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "compare_deploys",
		Description: "Diff two of an app's deploy attempts (from/to deploy attempt IDs), or from against the app's current live desired state when to is omitted. Only image, commit SHA, source, and status are ever snapshotted per attempt; env vars, ports, domains, and resource limits are not tracked per attempt and come back listed as unsnapshotted, not diffed. Read-only.",
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
