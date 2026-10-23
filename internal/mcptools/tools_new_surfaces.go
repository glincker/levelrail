package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type imageInventoryInput struct {
	Node       string `json:"node,omitempty" jsonschema:"node id; omit for the local node"`
	KeepPerApp *int   `json:"keep_per_app,omitempty" jsonschema:"retention override, images kept per app; omit for the server default"`
	LayersOf   string `json:"layers_of,omitempty" jsonschema:"an image id from the inventory; returns its layer breakdown instead of the inventory"`
}

type imageInventoryOutput struct {
	Inventory *apiclient.ImageListResource   `json:"inventory,omitempty"`
	Layers    *apiclient.ImageLayersResource `json:"layers,omitempty"`
}

type imageGCInput struct {
	Node       string `json:"node,omitempty" jsonschema:"node id; omit for the local node"`
	KeepPerApp *int   `json:"keep_per_app,omitempty" jsonschema:"images kept per app"`
	MinAgeHrs  *int   `json:"min_age_hours,omitempty" jsonschema:"minimum image age in hours before removal"`
}

type listFunctionsInput struct {
	Window   string `json:"window,omitempty" jsonschema:"metrics window: 1h, 24h (default) or 7d"`
	Runtimes bool   `json:"runtimes,omitempty" jsonschema:"list the starter runtimes instead of the functions"`
}

type listFunctionsOutput struct {
	Functions *apiclient.FunctionsList    `json:"functions,omitempty"`
	Runtimes  *apiclient.FunctionRuntimes `json:"runtimes,omitempty"`
}

type listBucketsInput struct {
	App   string `json:"app,omitempty" jsonschema:"only buckets bound to this app"`
	Usage bool   `json:"usage,omitempty" jsonschema:"also fetch stored bytes and object count per bucket"`
}

type bucketWithUsage struct {
	apiclient.StorageBucket
	Usage *apiclient.BucketUsage `json:"usage,omitempty"`
}

type createBucketInput struct {
	Name          string `json:"name" jsonschema:"bucket name"`
	TargetID      string `json:"target_id" jsonschema:"storage destination id"`
	AppName       string `json:"app_name,omitempty" jsonschema:"bind to this app on creation"`
	EnvPrefix     string `json:"env_prefix,omitempty" jsonschema:"env var prefix for the bound app"`
	Public        bool   `json:"public,omitempty"`
	QuotaBytes    int64  `json:"quota_bytes,omitempty"`
	LifecycleDays int    `json:"lifecycle_days,omitempty"`
}

type bindBucketInput struct {
	App       string `json:"app" jsonschema:"app name"`
	BucketID  string `json:"bucket_id" jsonschema:"bucket id"`
	EnvPrefix string `json:"env_prefix,omitempty" jsonschema:"env var prefix injected into the app"`
}

type presignInput struct {
	BucketID       string `json:"bucket_id" jsonschema:"bucket id"`
	Key            string `json:"key" jsonschema:"object key"`
	Method         string `json:"method,omitempty" jsonschema:"GET (default) or PUT"`
	ExpiresSeconds int    `json:"expires_seconds,omitempty" jsonschema:"URL lifetime in seconds"`
}

type linkDatabaseInput struct {
	Database string `json:"database" jsonschema:"database name"`
	App      string `json:"app" jsonschema:"app that receives the connection env var"`
	EnvVar   string `json:"env_var,omitempty" jsonschema:"env var name, default derived from the database"`
	Field    string `json:"field,omitempty" jsonschema:"connection field to inject, default the full URL"`
}

type unbindResult struct {
	Unbound bool `json:"unbound"`
}

type noInput struct{}

type listZonesInput struct {
	NodeID   string `json:"node_id,omitempty"`
	Capacity bool   `json:"capacity,omitempty"`
}

type listZonesOutput struct {
	Zones      []apiclient.ZoneSummary   `json:"zones"`
	Nodes      []apiclient.ZoneNode      `json:"nodes"`
	Labeled    bool                      `json:"labeled"`
	Domains    *apiclient.NodeDomains    `json:"domains,omitempty"`
	Onboarding *apiclient.NodeOnboarding `json:"onboarding,omitempty"`
}

func registerNewSurfaceTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_image_inventory",
		Description: "List images on a node with owners, size and retention protection, or with layers_of the layer breakdown of one image.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in imageInventoryInput) (*mcp.CallToolResult, imageInventoryOutput, error) {
		if in.LayersOf != "" {
			l, err := client.GetImageLayers(ctx, in.Node, in.LayersOf)
			if err != nil {
				return nil, imageInventoryOutput{}, fmt.Errorf("image layers %q: %w", in.LayersOf, err)
			}
			return nil, imageInventoryOutput{Layers: &l}, nil
		}
		keep := -1
		if in.KeepPerApp != nil {
			keep = *in.KeepPerApp
		}
		inv, err := client.ListImages(ctx, in.Node, keep)
		if err != nil {
			return nil, imageInventoryOutput{}, fmt.Errorf("list images: %w", err)
		}
		return nil, imageInventoryOutput{Inventory: &inv}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "preview_image_gc",
		Description: "Dry run of image garbage collection: the images retention would remove and the bytes it would reclaim. Changes nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in imageGCInput) (*mcp.CallToolResult, apiclient.ImageGCResource, error) {
		out, err := client.RunImageGC(ctx, apiclient.ImageGCRequest{Node: in.Node, KeepPerApp: in.KeepPerApp, MinAgeHrs: in.MinAgeHrs, DryRun: true})
		if err != nil {
			return nil, apiclient.ImageGCResource{}, fmt.Errorf("preview image gc: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "apply_image_gc",
		Description: "Remove images that image retention marks as candidates (run preview_image_gc first). Needs a root token, like the REST API; rollback targets stay protected.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in imageGCInput) (*mcp.CallToolResult, apiclient.ImageGCResource, error) {
		out, err := client.RunImageGC(ctx, apiclient.ImageGCRequest{Node: in.Node, KeepPerApp: in.KeepPerApp, MinAgeHrs: in.MinAgeHrs})
		if err != nil {
			return nil, apiclient.ImageGCResource{}, fmt.Errorf("apply image gc: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_functions",
		Description: "List scale-to-zero functions with sleeping state, invocations, 5xx rate and p95 over a window, or with runtimes the starter runtimes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listFunctionsInput) (*mcp.CallToolResult, listFunctionsOutput, error) {
		if in.Runtimes {
			rt, err := client.ListFunctionRuntimes(ctx)
			if err != nil {
				return nil, listFunctionsOutput{}, fmt.Errorf("list function runtimes: %w", err)
			}
			return nil, listFunctionsOutput{Runtimes: &rt}, nil
		}
		fl, err := client.ListFunctions(ctx, in.Window)
		if err != nil {
			return nil, listFunctionsOutput{}, fmt.Errorf("list functions: %w", err)
		}
		return nil, listFunctionsOutput{Functions: &fl}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_zones",
		Description: "List failure domains with nodes and workloads. capacity adds totals per provider, region, zone and single-zone warnings; node_id gives preflight checks and fixes. Root token.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listZonesInput) (*mcp.CallToolResult, listZonesOutput, error) {
		if in.NodeID != "" {
			ob, err := client.GetNodeOnboarding(ctx, in.NodeID)
			if err != nil {
				return nil, listZonesOutput{}, fmt.Errorf("get node onboarding: %w", err)
			}
			return nil, listZonesOutput{Onboarding: &ob}, nil
		}
		z, err := client.GetZones(ctx)
		if err != nil {
			return nil, listZonesOutput{}, fmt.Errorf("list zones: %w", err)
		}
		out := listZonesOutput{Zones: z.Zones, Nodes: z.Nodes, Labeled: z.Labeled}
		if in.Capacity {
			d, err := client.GetNodeDomains(ctx)
			if err != nil {
				return nil, listZonesOutput{}, fmt.Errorf("get node domains: %w", err)
			}
			out.Domains = &d
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_storage_buckets",
		Description: "List storage buckets with their destination, app bindings and env keys, optionally with stored size and object count. No credentials.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listBucketsInput) (*mcp.CallToolResult, []bucketWithUsage, error) {
		buckets, err := client.ListStorageBuckets(ctx, in.App)
		if err != nil {
			return nil, nil, fmt.Errorf("list storage buckets: %w", err)
		}
		out := make([]bucketWithUsage, 0, len(buckets))
		for _, b := range buckets {
			row := bucketWithUsage{StorageBucket: b}
			if in.Usage {
				u, err := client.BucketUsage(ctx, b.ID)
				if err != nil {
					return nil, nil, fmt.Errorf("usage of bucket %q: %w", b.ID, err)
				}
				row.Usage = &u
			}
			out = append(out, row)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "create_storage_bucket",
		Description: "Create a bucket on a storage destination, optionally binding it to an app (injects S3 env vars on the next deploy). Returns env key names, never values.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createBucketInput) (*mcp.CallToolResult, apiclient.StorageBucket, error) {
		b, err := client.CreateStorageBucket(ctx, apiclient.CreateStorageBucketRequest{Name: in.Name, TargetID: in.TargetID, AppName: in.AppName, EnvPrefix: in.EnvPrefix, Public: in.Public, QuotaBytes: in.QuotaBytes, LifecycleDays: in.LifecycleDays})
		if err != nil {
			return nil, apiclient.StorageBucket{}, fmt.Errorf("create storage bucket %q: %w", in.Name, err)
		}
		return nil, b, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "bind_storage_bucket",
		Description: "Bind a bucket to an app so its env vars are injected on the next deploy.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bindBucketInput) (*mcp.CallToolResult, apiclient.StorageBucket, error) {
		b, err := client.BindAppBucket(ctx, in.App, in.BucketID, in.EnvPrefix)
		if err != nil {
			return nil, apiclient.StorageBucket{}, fmt.Errorf("bind bucket %q to app %q: %w", in.BucketID, in.App, err)
		}
		return nil, b, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "unbind_storage_bucket",
		Description: "Remove a bucket binding from an app. The bucket and its objects are kept.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bindBucketInput) (*mcp.CallToolResult, unbindResult, error) {
		if err := client.UnbindAppBucket(ctx, in.App, in.BucketID); err != nil {
			return nil, unbindResult{}, fmt.Errorf("unbind bucket %q from app %q: %w", in.BucketID, in.App, err)
		}
		return nil, unbindResult{Unbound: true}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "presign_bucket_object",
		Description: "Mint a time-limited URL to GET or PUT one bucket object. The URL is a bearer credential: do not log or share it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in presignInput) (*mcp.CallToolResult, apiclient.BucketPresign, error) {
		p, err := client.PresignBucketObject(ctx, in.BucketID, in.Key, in.Method, in.ExpiresSeconds)
		if err != nil {
			return nil, apiclient.BucketPresign{}, fmt.Errorf("presign %q in bucket %q: %w", in.Key, in.BucketID, err)
		}
		return nil, p, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_database_links",
		Description: "List the apps a database injects a connection env var into, with the env var and field.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseNameInput) (*mcp.CallToolResult, []apiclient.DatabaseLink, error) {
		links, err := client.ListDatabaseLinks(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list links of database %q: %w", in.Name, err)
		}
		return nil, links, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "link_database_app",
		Description: "Link an app to a database so a connection env var is injected on the next deploy. Reports host, mesh DNS and whether the link crosses nodes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in linkDatabaseInput) (*mcp.CallToolResult, apiclient.AppConnectionResource, error) {
		out, err := client.ConnectDatabaseApp(ctx, in.Database, in.App, in.EnvVar, in.Field)
		if err != nil {
			return nil, apiclient.AppConnectionResource{}, fmt.Errorf("link app %q to database %q: %w", in.App, in.Database, err)
		}
		return nil, out, nil
	})
}
