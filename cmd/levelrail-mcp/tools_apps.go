package main

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultLogsWindow mirrors cmd/levelrail-cli's own default lookback
// window for "apps logs" (apps_logs.go's defaultLogsWindow): what
// get_app_logs searches when the caller doesn't set since.
const defaultLogsWindow = time.Hour

// defaultLogTail and maxLogTail bound get_app_logs' response: an MCP
// tool call returns one response, not a stream, so this is a real
// search plus a hard client-side cap, never an unbounded dump into the
// calling agent's context.
const (
	defaultLogTail = 200
	maxLogTail     = 200
)

func registerAppTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_apps",
		Description: "List every app on the control plane: name, image, port, domains, and resource limits.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.AppResource, error) {
		apps, err := client.ListApps(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list apps: %w", err)
		}
		return nil, apps, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app",
		Description: "Get one app's current desired state: image, port, domains, env, resources, health checks.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.GetApp(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("get app %q: %w", in.Name, err)
		}
		return nil, app, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "deploy_app",
		Description: "Point an existing app's desired image at a new tag. Asynchronous: returns once the desired state is saved, not once the new container is actually running; use get_app_status to watch it converge. Deploying an older, already-known tag is how a rollback is done.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployAppInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.DeployApp(ctx, in.Name, in.Image)
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("deploy app %q to image %q: %w", in.Name, in.Image, err)
		}
		return nil, app, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "deploy_compose",
		Description: "Deploy a Docker Compose YAML document as an app: one member service per compose service, all created in a single synchronous call.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployComposeInput) (*mcp.CallToolResult, apiclient.ComposeDeployResult, error) {
		result, err := client.DeployCompose(ctx, in.Name, []byte(in.Compose))
		if err != nil {
			return nil, apiclient.ComposeDeployResult{}, fmt.Errorf("deploy compose to app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rollback_app",
		Description: "Point an existing app's desired image back at an older, already-built image tag. Identical request to deploy_app (there is no separate rollback endpoint server-side, matching how cmd/levelrail-cli's own 'apps rollback' and the web dashboard's 'Rollback to this build' button both work); given as its own tool so a rollback intent doesn't have to be expressed by re-purposing deploy_app. Asynchronous: use get_app_status to watch it converge.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployAppInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.DeployApp(ctx, in.Name, in.Image)
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("roll back app %q to image %q: %w", in.Name, in.Image, err)
		}
		return nil, app, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "restart_app",
		Description: "Force an app's running container to be recreated with no image change.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.RestartApp(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("restart app %q: %w", in.Name, err)
		}
		return nil, app, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_status",
		Description: "Get an app's current stored reconcile conditions (type, status, reason, message) from the application controller. This is current status, not a historical log.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.ConditionResource, error) {
		conditions, err := client.GetDeployStatus(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get status for app %q: %w", in.Name, err)
		}
		return nil, conditions, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_deploys",
		Description: "List an app's deploy/reconcile conditions. Same underlying data as get_app_status: the control plane has no separate deploy history log, only current reconcile conditions.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.ConditionResource, error) {
		conditions, err := client.GetDeployStatus(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploys for app %q: %w", in.Name, err)
		}
		return nil, conditions, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_logs",
		Description: "Search an app's already-stored log entries in a time window. A bounded historical search, not a live tail: at most 200 entries are returned per call.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appLogsInput) (*mcp.CallToolResult, []apiclient.LogEntryResource, error) {
		window := defaultLogsWindow
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, nil, fmt.Errorf("get logs for app %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			window = d
		}
		to := time.Now()
		from := to.Add(-window)

		entries, err := client.QueryLogs(ctx, in.Name, from, to, in.Query)
		if err != nil {
			return nil, nil, fmt.Errorf("get logs for app %q: %w", in.Name, err)
		}
		return nil, tailLogEntries(entries, in.Tail), nil
	})
}

// tailLogEntries applies get_app_logs' client-side "last N entries"
// bound: tail<=0 falls back to defaultLogTail, and any value above
// maxLogTail is clamped to it, so this tool's response is bounded
// regardless of what the caller asks for.
func tailLogEntries(entries []apiclient.LogEntryResource, tail int) []apiclient.LogEntryResource {
	switch {
	case tail <= 0:
		tail = defaultLogTail
	case tail > maxLogTail:
		tail = maxLogTail
	}
	if len(entries) > tail {
		return entries[len(entries)-tail:]
	}
	return entries
}

type appNameInput struct {
	Name string `json:"name" jsonschema:"the app's name"`
}

type deployAppInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Image string `json:"image" jsonschema:"image reference to deploy, e.g. registry.example.com/org/app:tag"`
}

type deployComposeInput struct {
	Name    string `json:"name" jsonschema:"the app's name to create or update"`
	Compose string `json:"compose" jsonschema:"the full Docker Compose YAML document, as raw text, not a file path"`
}

type appLogsInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Since string `json:"since,omitempty" jsonschema:"how far back to search, e.g. '1h', '30m'; default 1h"`
	Query string `json:"query,omitempty" jsonschema:"full-text search phrase; empty matches every line in the window"`
	Tail  int    `json:"tail,omitempty" jsonschema:"max number of most-recent entries to return, default and hard cap 200"`
}
