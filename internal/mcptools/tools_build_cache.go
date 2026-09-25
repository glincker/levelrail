package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type getBuildCacheInput struct {
	AppName string `json:"app_name,omitempty" jsonschema:"app to show; omit for the global default and every per-app setting"`
	Stats   bool   `json:"stats,omitempty" jsonschema:"also list the app's cache prefix in the bucket for object count, size and last export time (needs app_name)"`
}

type getBuildCacheOutput struct {
	Settings []apiclient.BuildCacheSetting `json:"settings"`
	Bucket   *apiclient.BuildCacheStats    `json:"bucket,omitempty"`
}

type setBuildCacheInput struct {
	AppName  string `json:"app_name,omitempty" jsonschema:"app to configure; omit to set the default for every app without its own setting"`
	TargetID string `json:"target_id" jsonschema:"storage destination id"`
	Mode     string `json:"mode,omitempty" jsonschema:"BuildKit cache export mode, min or max (default max)"`
	Enabled  *bool  `json:"enabled,omitempty" jsonschema:"false keeps the setting but stops using the cache (default true)"`
}

func registerBuildCacheTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_build_cache",
		Description: "Show BuildKit remote build cache settings: which storage destination an app (or every app by default) caches layers in, the export mode, the last build outcome and any warning from a build that fell back to no cache. With stats, also object count, size and last export time from the bucket. No credentials, ever. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getBuildCacheInput) (*mcp.CallToolResult, getBuildCacheOutput, error) {
		settings, err := client.ListBuildCache(ctx)
		if err != nil {
			return nil, getBuildCacheOutput{}, fmt.Errorf("list build cache settings: %w", err)
		}
		out := getBuildCacheOutput{Settings: []apiclient.BuildCacheSetting{}}
		for _, s := range settings {
			if in.AppName == "" || s.AppName == in.AppName {
				out.Settings = append(out.Settings, s)
			}
		}
		if in.Stats && in.AppName != "" {
			stats, err := client.GetBuildCacheStats(ctx, in.AppName)
			if err != nil {
				return nil, getBuildCacheOutput{}, fmt.Errorf("build cache stats for %q: %w", in.AppName, err)
			}
			out.Bucket = &stats
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_build_cache",
		Description: "Point an app's BuildKit remote build cache (or the default for every app) at a storage destination. Layers are stored under build-cache/<app>/. A cache failure never fails a build, it is recorded as a warning. Takes a destination id only, never bucket credentials.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setBuildCacheInput) (*mcp.CallToolResult, apiclient.BuildCacheSetting, error) {
		out, err := client.SetBuildCache(ctx, apiclient.BuildCacheRequest{AppName: in.AppName, TargetID: in.TargetID, Mode: in.Mode, Enabled: in.Enabled})
		if err != nil {
			return nil, apiclient.BuildCacheSetting{}, fmt.Errorf("set build cache: %w", err)
		}
		return nil, out, nil
	})
}
