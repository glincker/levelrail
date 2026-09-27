package mcptools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// agentCoreTools is the exact tool set the agent-core mode exposes.
var agentCoreTools = map[string]bool{
	"list_apps": true, "get_app": true, "get_app_status": true, "list_deploy_attempts": true,
	"deploy_app": true, "rollback_app": true, "restart_app": true, "get_app_logs": true,
	"diagnose_app_failure": true, "list_app_images": true, "get_app_env": true, "set_app_env": true,
	"set_app_secret": true, "set_app_domains": true, "check_domain_dns": true,
}

const (
	coreClipMessage  = 240
	coreDefaultTries = 10
	coreMaxTries     = 30
	coreMaxTags      = 20
	coreMaxLogBytes  = 8 << 10
)

type coreAttemptsInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Limit int    `json:"limit,omitempty" jsonschema:"newest attempts to return, default 10, max 30"`
}

type coreCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type coreDeployResult struct {
	Name            string `json:"name"`
	Image           string `json:"image,omitempty"`
	Accepted        bool   `json:"accepted"`
	PendingApproval string `json:"pending_approval_id,omitempty"`
	Next            string `json:"next"`
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "..."
}

func coreDeploy(ctx context.Context, client *apiclient.Client, verb string, in deployAppInput) (any, error) {
	res, err := client.DeployApp(ctx, in.Name, in.Image, in.Confirm)
	if err != nil {
		return nil, fmt.Errorf("%s app %q to image %q: %w", verb, in.Name, in.Image, err)
	}
	out := coreDeployResult{Name: in.Name, Image: in.Image, Accepted: true, Next: "call get_app_status to watch it converge"}
	if res.PendingApproval != nil {
		out.Accepted = false
		out.PendingApproval = res.PendingApproval.ID
		out.Next = "a different privileged human must approve before it deploys"
	}
	return out, nil
}

func registerAgentCoreTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_apps",
		Description: "List all apps with image, port and domains. Start here to find an app name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		apps, err := client.ListApps(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list apps: %w", err)
		}
		rows := make([]map[string]any, 0, len(apps))
		for _, a := range apps {
			rows = append(rows, map[string]any{"name": a.Name, "image": a.Image, "port": a.Port, "domains": a.Domains})
		}
		return nil, map[string]any{"count": len(rows), "apps": rows}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app",
		Description: "Get one app's desired state: image, port, domains and env key names. Use get_app_env for values.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, any, error) {
		a, err := client.GetApp(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get app %q: %w", in.Name, err)
		}
		envKeys := make([]string, 0, len(a.Env))
		for k := range a.Env {
			envKeys = append(envKeys, k)
		}
		slices.Sort(envKeys)
		return nil, map[string]any{"name": a.Name, "image": a.Image, "port": a.Port, "domains": a.Domains, "env_keys": envKeys, "secret_keys": a.SecretEnv}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_status",
		Description: "Get an app's current reconcile conditions. Call after deploy_app, rollback_app or restart_app to see if it converged.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, any, error) {
		conds, err := client.GetDeployStatus(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get status for app %q: %w", in.Name, err)
		}
		rows := make([]coreCondition, 0, len(conds))
		for _, c := range conds {
			rows = append(rows, coreCondition{Type: c.Type, Status: c.Status, Reason: c.Reason, Message: clip(c.Message, coreClipMessage)})
		}
		return nil, map[string]any{"name": in.Name, "conditions": rows}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_deploy_attempts",
		Description: "List an app's newest deploy attempts with status, image, commit and error. Use to find what failed or which image to roll back to.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in coreAttemptsInput) (*mcp.CallToolResult, any, error) {
		attempts, err := client.ListDeployAttempts(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploy attempts for app %q: %w", in.Name, err)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = coreDefaultTries
		}
		limit = min(limit, coreMaxTries, len(attempts))
		rows := make([]map[string]any, 0, limit)
		for _, a := range attempts[:limit] {
			row := map[string]any{"id": a.ID, "status": a.Status, "image": a.Image, "started_at": a.StartedAt.UTC().Format(time.RFC3339)}
			if a.CommitSHA != "" {
				row["commit"] = a.CommitSHA
			}
			if a.Error != "" {
				row["error"] = clip(a.Error, coreClipMessage)
			}
			rows = append(rows, row)
		}
		return nil, map[string]any{"total": len(attempts), "shown": len(rows), "attempts": rows}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "deploy_app",
		Description: "Deploy a new image tag to an existing app. Asynchronous: follow with get_app_status. Protected environments need confirm true and a human approval.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployAppInput) (*mcp.CallToolResult, any, error) {
		out, err := coreDeploy(ctx, client, "deploy", in)
		return nil, out, err
	})

	addTool(server, &mcp.Tool{
		Name:        "rollback_app",
		Description: "Roll an app back to an older image tag (find one with list_deploy_attempts or list_app_images). Same approval gate as deploy_app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployAppInput) (*mcp.CallToolResult, any, error) {
		out, err := coreDeploy(ctx, client, "roll back", in)
		return nil, out, err
	})

	addTool(server, &mcp.Tool{
		Name:        "restart_app",
		Description: "Recreate an app's container with no image change, e.g. to pick up new env vars.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, any, error) {
		if _, err := client.RestartApp(ctx, in.Name); err != nil {
			return nil, nil, fmt.Errorf("restart app %q: %w", in.Name, err)
		}
		return nil, map[string]any{"name": in.Name, "restarted": true, "next": "call get_app_status to watch it converge"}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_logs",
		Description: "Search an app's stored logs (newest last, capped at 8 KB). Narrow with since (e.g. 30m) and query.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appLogsInput) (*mcp.CallToolResult, any, error) {
		window := defaultLogsWindow
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, nil, fmt.Errorf("get logs for app %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			window = d
		}
		to := time.Now()
		entries, err := client.QueryLogs(ctx, in.Name, to.Add(-window), to, in.Query)
		if err != nil {
			return nil, nil, fmt.Errorf("get logs for app %q: %w", in.Name, err)
		}
		return nil, coreLogExcerpt(tailLogEntries(entries, in.Tail), len(entries)), nil
	})

	addTool(server, &mcp.Tool{
		Name:        "diagnose_app_failure",
		Description: "Explain why an app's latest deploy failed or why it is crashlooping, with causes and suggested fixes. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in diagnoseAppInput) (*mcp.CallToolResult, any, error) {
		d, err := client.DiagnoseApp(ctx, in.Name, in.DeployID)
		if err != nil {
			return nil, nil, fmt.Errorf("diagnose app %q: %w", in.Name, err)
		}
		causes := make([]map[string]any, 0, len(d.Causes))
		for _, c := range d.Causes {
			fixes := make([]string, 0, len(c.Fixes))
			for _, f := range c.Fixes {
				fixes = append(fixes, fmt.Sprintf("%d: %s", f.N, f.Label))
			}
			causes = append(causes, map[string]any{"code": c.Code, "confidence": c.Confidence, "why": clip(c.Explanation, coreClipMessage), "fixes": fixes})
		}
		return nil, map[string]any{"explanation": clip(d.Explanation, 400), "suggestion": clip(d.Suggestion, 400), "confidence": d.Confidence, "causes": causes}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_app_images",
		Description: "List image tags present locally for an app, newest first. Use to pick a rollback target.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, any, error) {
		images, err := client.ListAppImages(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list images for app %q: %w", in.Name, err)
		}
		tags := make([]string, 0, min(len(images), coreMaxTags))
		for _, im := range images[:min(len(images), coreMaxTags)] {
			tags = append(tags, im.Tag)
		}
		return nil, map[string]any{"total": len(images), "tags": tags}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "set_app_domains",
		Description: "Replace an app's whole domain list (include existing domains you want to keep). A domain owned by another app is refused.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAppDomainsInput) (*mcp.CallToolResult, any, error) {
		domains := make([]string, 0, len(in.Domains))
		for _, d := range in.Domains {
			if d = strings.ToLower(strings.TrimSpace(d)); d != "" && !slices.Contains(domains, d) {
				domains = append(domains, d)
			}
		}
		res, err := client.EditAppDomains(ctx, in.Name, apiclient.EditDomainsRequest{Set: &domains})
		if err != nil {
			return nil, nil, fmt.Errorf("set domains for app %q: %w", in.Name, err)
		}
		return nil, map[string]any{"name": res.App, "domains": res.Domains, "changed": res.Changed}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "check_domain_dns",
		Description: "Check whether one of an app's domains resolves to this server. Use when a domain does not reach its app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, any, error) {
		r, err := client.CheckDomain(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, nil, fmt.Errorf("check dns for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, map[string]any{"domain": r.Domain, "status": r.Status, "expected_host": r.ExpectedHost, "resolved_hosts": r.ResolvedHosts}, nil
	})
}

// coreLogExcerpt renders entries as "time message" lines, dropping the
// oldest lines once the byte cap is reached.
func coreLogExcerpt(entries []apiclient.LogEntryResource, matched int) map[string]any {
	lines := make([]string, 0, len(entries))
	size := 0
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		line := e.Timestamp.UTC().Format("15:04:05") + " " + clip(e.Message, 500)
		if size+len(line) > coreMaxLogBytes {
			break
		}
		size += len(line)
		lines = append(lines, line)
	}
	slices.Reverse(lines)
	out := map[string]any{"matched": matched, "shown": len(lines), "lines": lines}
	if len(lines) < matched {
		out["notice"] = "output capped, narrow with since or query"
	}
	return out
}
