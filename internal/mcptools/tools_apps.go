package mcptools

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

const (
	defaultAppsPage = 50
	maxAppsPage     = 200
)

type listAppsInput struct {
	Limit       int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
	Offset      int    `json:"offset,omitempty"`
	Environment string `json:"environment,omitempty" jsonschema:"only apps in this environment"`
	Fields      string `json:"fields,omitempty" jsonschema:"comma-separated keys to keep per item"`
}

type listAppsOutput struct {
	Apps       []apiclient.AppResource `json:"apps"`
	Total      int                     `json:"total"`
	NextOffset int                     `json:"next_offset,omitempty"`
}

func registerAppTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_apps",
		Description: "List apps (name, image, port, domains, limits), 50 per page. Pass next_offset as offset for more.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listAppsInput) (*mcp.CallToolResult, listAppsOutput, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = defaultAppsPage
		}
		limit = min(limit, maxAppsPage)
		page, err := client.ListAppsPage(ctx, apiclient.ListAppsOptions{Limit: limit, Offset: max(in.Offset, 0), Environment: in.Environment})
		if err != nil {
			return nil, listAppsOutput{}, fmt.Errorf("list apps: %w", err)
		}
		return nil, listAppsOutput{Apps: page.Items, Total: page.TotalCount, NextOffset: page.NextOffset}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app",
		Description: "Get one app's current desired state: image, port, domains, env, resources, health checks, command override, volumes, bind mounts.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.GetApp(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("get app %q: %w", in.Name, err)
		}
		return nil, app, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "deploy_app",
		Description: "Point an app at a new image tag. Asynchronous: watch with get_app_status. Protected environments need confirm and yield pending_approval until another human approves.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployAppInput) (*mcp.CallToolResult, apiclient.DeployTriggerResult, error) {
		result, err := client.DeployApp(ctx, in.Name, in.Image, in.Confirm)
		if err != nil {
			return nil, apiclient.DeployTriggerResult{}, fmt.Errorf("deploy app %q to image %q: %w", in.Name, in.Image, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "deploy_compose",
		Description: "Deploy a Docker Compose YAML document as an app: one member service per compose service, all created in a single synchronous call.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployComposeInput) (*mcp.CallToolResult, apiclient.ComposeDeployResult, error) {
		result, err := client.DeployCompose(ctx, in.Name, []byte(in.Compose))
		if err != nil {
			return nil, apiclient.ComposeDeployResult{}, fmt.Errorf("deploy compose to app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "rollback_app",
		Description: "Point an app back at an older image tag. Same approval gate as deploy_app, asynchronous. Refuses an image that was never a deploy of the app and lists the valid targets. Does not verify the tag is unchanged; prefer rollback_app_to_deploy when a deploy_id is known.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployAppInput) (*mcp.CallToolResult, apiclient.DeployTriggerResult, error) {
		result, err := client.DeployAppWith(ctx, in.Name, apiclient.DeployTriggerRequest{Image: in.Image, Confirm: in.Confirm, Rollback: true})
		if err != nil {
			return nil, apiclient.DeployTriggerResult{}, fmt.Errorf("roll back app %q to image %q: %w", in.Name, in.Image, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "rollback_app_to_deploy",
		Description: "Roll back to a past succeeded attempt (from list_deploy_attempts), pinned by its recorded image digest. Fails closed if the image was garbage collected or its tag changed. Same freeze and approval gate as deploy_app. Asynchronous: watch with get_app_status.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in rollbackToDeployInput) (*mcp.CallToolResult, apiclient.DeployTriggerResult, error) {
		result, err := client.RollbackToDeploy(ctx, in.Name, in.DeployID, apiclient.RollbackToRequest{
			Confirm:        in.Confirm,
			OverrideFreeze: in.OverrideFreeze,
			OverrideReason: in.OverrideReason,
		})
		if err != nil {
			return nil, apiclient.DeployTriggerResult{}, fmt.Errorf("roll back app %q to deploy %q: %w", in.Name, in.DeployID, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "clone_app",
		Description: "Duplicate an app under a new name: image, port, env, resources, health checks, strategy and replicas. Domains stay empty unless domain_suffix derives them; secret values copy only with copy_secrets (needs read:sensitive); volume data never copies. Fails on a name conflict.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cloneAppInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.CloneAppWith(ctx, in.Name, apiclient.CloneAppRequest{NewName: in.NewName, CopySecrets: in.CopySecrets, DomainSuffix: in.DomainSuffix, EnvironmentID: in.EnvironmentID, Domains: cloneDomainMode(in.DomainSuffix)})
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("clone app %q to %q: %w", in.Name, in.NewName, err)
		}
		return nil, app, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_app_images",
		Description: "List locally-present image tags under an app's current image repo, newest first, for deploy_app or rollback_app. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.ImageResource, error) {
		images, err := client.ListAppImages(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list images for app %q: %w", in.Name, err)
		}
		return nil, images, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "restart_app",
		Description: "Force an app's running container to be recreated with no image change.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.RestartApp(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("restart app %q: %w", in.Name, err)
		}
		return nil, app, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_status",
		Description: "Get an app's current reconcile conditions (type, status, reason, message). Current status only, not history.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.ConditionResource, error) {
		conditions, err := client.GetDeployStatus(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get status for app %q: %w", in.Name, err)
		}
		return nil, conditions, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_deploys",
		Description: "List an app's deploy and reconcile conditions. Same data as get_app_status: there is no separate deploy history.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.ConditionResource, error) {
		conditions, err := client.GetDeployStatus(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploys for app %q: %w", in.Name, err)
		}
		return nil, conditions, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_logs",
		Description: "Search an app's stored log entries in a time window. Historical, not a live tail: at most 200 entries per call.",
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

	addTool(server, &mcp.Tool{
		Name:        "get_deploy",
		Description: "One deploy attempt (deploy_id or \"latest\"): status, image, commit and, on failure, the structured failure with cause, step, log excerpt and suggested fix.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDeployInput) (*mcp.CallToolResult, apiclient.DeployAttemptResource, error) {
		id := in.DeployID
		if id == "" {
			id = "latest"
		}
		attempt, err := client.GetDeploy(ctx, in.Name, id)
		if err != nil {
			return nil, apiclient.DeployAttemptResource{}, fmt.Errorf("get deploy %q for app %q: %w", id, in.Name, err)
		}
		return nil, attempt, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_deploy_attempts",
		Description: "An app's deploy attempts, newest first: trigger, status, image, commit, times. A log of what was tried, unlike get_app_status.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listDeployAttemptsInput) (*mcp.CallToolResult, []apiclient.DeployAttemptResource, error) {
		attempts, err := client.ListDeployAttemptsFiltered(ctx, in.Name, apiclient.DeployAttemptFilter{Branch: in.Branch, Author: in.Author, Commit: in.Commit, Status: in.Status, Query: in.Query, Limit: in.Limit})
		if err != nil {
			return nil, nil, fmt.Errorf("list deploy attempts for app %q: %w", in.Name, err)
		}
		return nil, attempts, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_deploy_sbom",
		Description: "Get one deploy's SBOM summary: package count, types, license counts, first packages. Defaults to the newest deploy with an SBOM. Exists only for Dockerfile builds with APP_BUILD_ATTEST=true.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployRefInput) (*mcp.CallToolResult, apiclient.SBOMSummary, error) {
		id, err := supplyChainDeploy(ctx, client, in, true)
		if err != nil {
			return nil, apiclient.SBOMSummary{}, err
		}
		sum, err := client.GetSBOM(ctx, in.Name, id)
		if err != nil {
			return nil, apiclient.SBOMSummary{}, fmt.Errorf("get sbom of deploy %q for app %q: %w", id, in.Name, err)
		}
		return nil, sum, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_deploy_vulnerabilities",
		Description: "Get one deploy's vulnerability scan: counts by severity, top fixable findings and the gate decision. Defaults to the newest scanned deploy; never starts a scan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployRefInput) (*mcp.CallToolResult, apiclient.VulnReport, error) {
		id, err := supplyChainDeploy(ctx, client, in, false)
		if err != nil {
			return nil, apiclient.VulnReport{}, err
		}
		rep, err := client.GetVulnerabilities(ctx, in.Name, id)
		if err != nil {
			return nil, apiclient.VulnReport{}, fmt.Errorf("get vulnerabilities of deploy %q for app %q: %w", id, in.Name, err)
		}
		return nil, rep, nil
	})
}

type deployRefInput struct {
	Name     string `json:"name" jsonschema:"the app's name"`
	DeployID string `json:"deploy_id,omitempty" jsonschema:"deploy attempt id; omit for the newest deploy with data"`
}

func supplyChainDeploy(ctx context.Context, client *apiclient.Client, in deployRefInput, needSBOM bool) (string, error) {
	if in.DeployID != "" {
		return in.DeployID, nil
	}
	attempts, err := client.ListDeployAttempts(ctx, in.Name)
	if err != nil {
		return "", fmt.Errorf("list deploy attempts for app %q: %w", in.Name, err)
	}
	for _, a := range attempts {
		if a.SBOMPackages != nil && (needSBOM || a.VulnCounts != nil) {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("no deploy of app %q has supply chain data yet", in.Name)
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

type listDeployAttemptsInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Branch string `json:"branch,omitempty"`
	Author string `json:"author,omitempty"`
	Commit string `json:"commit,omitempty" jsonschema:"commit SHA or prefix"`
	Status string `json:"status,omitempty" jsonschema:"attempt status, e.g. succeeded or failed"`
	Query  string `json:"query,omitempty" jsonschema:"free text match"`
	Limit  int    `json:"limit,omitempty"`
	Fields string `json:"fields,omitempty" jsonschema:"comma-separated keys to keep per item"`
}

type appNameInput struct {
	Name string `json:"name" jsonschema:"the app's name"`
}

type getDeployInput struct {
	Name     string `json:"name" jsonschema:"the app's name"`
	DeployID string `json:"deploy_id,omitempty" jsonschema:"the deploy attempt id, or latest (default)"`
}

type cloneAppInput struct {
	Name          string `json:"name" jsonschema:"the app to duplicate"`
	NewName       string `json:"new_name" jsonschema:"name for the new, cloned app"`
	CopySecrets   bool   `json:"copy_secrets,omitempty" jsonschema:"copy secret values, re-encrypted for the new app; needs read:sensitive"`
	DomainSuffix  string `json:"domain_suffix,omitempty" jsonschema:"derive domains by adding -SUFFIX to each source domain's first label; omit for no domains"`
	EnvironmentID string `json:"environment_id,omitempty" jsonschema:"environment of the source's project to place the clone in"`
}

type deployAppInput struct {
	Name    string `json:"name" jsonschema:"the app's name"`
	Image   string `json:"image" jsonschema:"image reference to deploy, e.g. registry.example.com/org/app:tag"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true to confirm a protected environment, else 409"`
}

type rollbackToDeployInput struct {
	Name           string `json:"name" jsonschema:"the app's name"`
	DeployID       string `json:"deploy_id" jsonschema:"the target deploy attempt's id, from list_deploy_attempts"`
	Confirm        bool   `json:"confirm,omitempty" jsonschema:"true to confirm a protected environment, else 409"`
	OverrideFreeze bool   `json:"override_freeze,omitempty" jsonschema:"roll back even though a deploy freeze window is active; requires override_reason"`
	OverrideReason string `json:"override_reason,omitempty" jsonschema:"why this rollback overrides an active freeze, recorded on the deploy"`
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

func cloneDomainMode(suffix string) string {
	if suffix == "" {
		return ""
	}
	return "suffix"
}
