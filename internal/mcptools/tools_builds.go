package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listBuildsInput struct {
	App    string `json:"app,omitempty" jsonschema:"only builds of this app"`
	Status string `json:"status,omitempty" jsonschema:"only this status: queued, running, succeeded, failed or canceled"`
	Limit  int    `json:"limit,omitempty" jsonschema:"most recent builds to load, default 100"`
	ID     string `json:"id,omitempty" jsonschema:"a build id; returns that one build with its failure hint instead of the list"`
}

type listBuildsOutput struct {
	Builds *apiclient.BuildListResource `json:"builds,omitempty"`
	Build  *apiclient.BuildResource     `json:"build,omitempty"`
}

type buildCacheInput struct {
	Node string `json:"node,omitempty" jsonschema:"node id; omit for the local node"`
}

type buildSpeedInput struct {
	App string `json:"app" jsonschema:"app name"`
}

type pruneBuildCacheInput struct {
	Node           string `json:"node,omitempty" jsonschema:"node id; omit for the local node"`
	OlderThanHours *int   `json:"older_than_hours,omitempty" jsonschema:"only cache idle at least this many hours, default 168"`
}

func registerBuildTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_builds",
		Description: "List the build queue and history across apps with duration, cache hit ratio, image size change, node and platform, or with id one build and its failure hint.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listBuildsInput) (*mcp.CallToolResult, listBuildsOutput, error) {
		if in.ID != "" {
			b, err := client.GetBuild(ctx, in.ID)
			if err != nil {
				return nil, listBuildsOutput{}, fmt.Errorf("get build %q: %w", in.ID, err)
			}
			return nil, listBuildsOutput{Build: &b}, nil
		}
		l, err := client.ListBuilds(ctx, in.App, in.Status, in.Limit)
		if err != nil {
			return nil, listBuildsOutput{}, fmt.Errorf("list builds: %w", err)
		}
		return nil, listBuildsOutput{Builds: &l}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_build_speed",
		Description: "Build speed of one app: time saved by cache (cold vs cached build median) and typed suggestions to speed builds up.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in buildSpeedInput) (*mcp.CallToolResult, apiclient.BuildSpeed, error) {
		out, err := client.GetBuildSpeed(ctx, in.App)
		if err != nil {
			return nil, apiclient.BuildSpeed{}, fmt.Errorf("get build speed: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_buildkit_cache",
		Description: "BuildKit cache usage of a node: total, in use and reclaimable bytes and the largest records.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in buildCacheInput) (*mcp.CallToolResult, apiclient.BuildCacheResource, error) {
		out, err := client.GetBuildCache(ctx, in.Node)
		if err != nil {
			return nil, apiclient.BuildCacheResource{}, fmt.Errorf("get build cache: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "preview_build_cache_prune",
		Description: "Dry run of a build cache prune: the idle cache records that would be removed and the bytes freed. Changes nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pruneBuildCacheInput) (*mcp.CallToolResult, apiclient.BuildCachePruneResource, error) {
		out, err := client.PruneBuildCache(ctx, apiclient.BuildCachePruneRequest{Node: in.Node, OlderThanHours: in.OlderThanHours})
		if err != nil {
			return nil, apiclient.BuildCachePruneResource{}, fmt.Errorf("preview build cache prune: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "apply_build_cache_prune",
		Description: "Remove idle BuildKit cache (run preview_build_cache_prune first). Cache an in-flight build holds is never removed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pruneBuildCacheInput) (*mcp.CallToolResult, apiclient.BuildCachePruneResource, error) {
		out, err := client.PruneBuildCache(ctx, apiclient.BuildCachePruneRequest{Node: in.Node, OlderThanHours: in.OlderThanHours, Apply: true})
		if err != nil {
			return nil, apiclient.BuildCachePruneResource{}, fmt.Errorf("apply build cache prune: %w", err)
		}
		return nil, out, nil
	})
}
