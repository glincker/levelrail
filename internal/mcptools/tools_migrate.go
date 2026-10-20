package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type importSessionBrief struct {
	ID        string         `json:"id"`
	Platform  string         `json:"platform"`
	Source    string         `json:"source"`
	Step      string         `json:"step"`
	Connected bool           `json:"connected"`
	States    map[string]int `json:"states"`
}

type importSessionsOutput struct {
	Sessions []importSessionBrief `json:"sessions"`
}

type cutoverItemInput struct {
	Session string `json:"session" jsonschema:"app import session id"`
	Item    string `json:"item" jsonschema:"source app id within the session"`
}

type cutoverCheckBrief struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type cutoverDomainBrief struct {
	Domain  string `json:"domain"`
	Method  string `json:"method"`
	Message string `json:"message,omitempty"`
}

type cutoverPlanOutput struct {
	App     string               `json:"app"`
	Verdict string               `json:"verdict"`
	Checks  []cutoverCheckBrief  `json:"checks"`
	Domains []cutoverDomainBrief `json:"domains"`
}

type cutoverRunBrief struct {
	ID           string `json:"id"`
	Mode         string `json:"mode"`
	State        string `json:"state"`
	Awaiting     string `json:"awaiting,omitempty"`
	Error        string `json:"error,omitempty"`
	Domains      int    `json:"domains"`
	Verified     int    `json:"verified"`
	Rollbackable bool   `json:"rollbackable"`
	UpdatedAt    string `json:"updated_at"`
}

type cutoverRunsOutput struct {
	Runs []cutoverRunBrief `json:"runs"`
}

type dataCopyBrief struct {
	Database   string `json:"database"`
	Engine     string `json:"engine"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Problem    string `json:"problem,omitempty"`
	NextAction string `json:"next_action,omitempty"`
	Checked    int    `json:"checked"`
	Mismatched int    `json:"mismatched"`
}

type dataCopiesOutput struct {
	Copies []dataCopyBrief `json:"copies"`
}

// registerMigrateTools is read only on purpose: starting, switching or rolling
// back a cutover and connecting to a source stay human-only operations.
func registerMigrateTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_app_import_sessions",
		Description: "App import sessions with step and per-state counts. See get_app_import_plan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, importSessionsOutput, error) {
		list, err := client.ListAppImportSessions(ctx)
		if err != nil {
			return nil, importSessionsOutput{}, fmt.Errorf("list app import sessions: %w", err)
		}
		out := importSessionsOutput{Sessions: make([]importSessionBrief, 0, len(list))}
		for _, v := range list {
			out.Sessions = append(out.Sessions, importSessionBrief{ID: v.ID, Platform: v.Platform, Source: v.SourceURL, Step: v.Step, Connected: v.Connected, States: v.States})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_import_cutover_plan",
		Description: "Cutover readiness of one staged import app: verdict, checks, per-domain DNS method.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cutoverItemInput) (*mcp.CallToolResult, cutoverPlanOutput, error) {
		p, err := client.AppImportCutoverPlan(ctx, in.Session, in.Item)
		if err != nil {
			return nil, cutoverPlanOutput{}, fmt.Errorf("get cutover plan %q/%q: %w", in.Session, in.Item, err)
		}
		out := cutoverPlanOutput{App: p.App, Verdict: p.Verdict, Checks: []cutoverCheckBrief{}, Domains: []cutoverDomainBrief{}}
		for _, c := range p.Checks {
			out.Checks = append(out.Checks, cutoverCheckBrief{ID: c.ID, Status: c.Status, Detail: c.Detail})
		}
		for _, d := range p.Domains {
			out.Domains = append(out.Domains, cutoverDomainBrief{Domain: d.Domain, Method: d.Method, Message: d.Message})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_app_import_cutover_runs",
		Description: "Cutover dry runs and switches of one import app, newest first, with state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cutoverItemInput) (*mcp.CallToolResult, cutoverRunsOutput, error) {
		runs, err := client.AppImportCutoverRuns(ctx, in.Session, in.Item)
		if err != nil {
			return nil, cutoverRunsOutput{}, fmt.Errorf("list cutover runs %q/%q: %w", in.Session, in.Item, err)
		}
		out := cutoverRunsOutput{Runs: make([]cutoverRunBrief, 0, len(runs))}
		for _, r := range runs {
			b := cutoverRunBrief{ID: r.ID, Mode: r.Mode, State: r.State, Awaiting: r.Awaiting, Error: r.Error, Domains: len(r.Domains), Rollbackable: r.Rollbackable, UpdatedAt: r.UpdatedAt.Format(time.RFC3339)}
			for _, d := range r.Domains {
				if d.Verified {
					b.Verified++
				}
			}
			out.Runs = append(out.Runs, b)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_database_data_copies",
		Description: "Status of each database data copy from a source platform, with mismatches.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataCopiesOutput, error) {
		list, err := client.ListDatabaseDataCopies(ctx)
		if err != nil {
			return nil, dataCopiesOutput{}, fmt.Errorf("list database data copies: %w", err)
		}
		out := dataCopiesOutput{Copies: make([]dataCopyBrief, 0, len(list))}
		for _, c := range list {
			b := dataCopyBrief{Database: c.Database, Engine: c.Engine, Status: c.Status, Reason: c.Reason, NextAction: c.NextAction, Checked: c.Checked, Mismatched: c.Mismatched}
			if c.Failure != nil {
				b.Problem = c.Failure.Summary
			}
			out.Copies = append(out.Copies, b)
		}
		return nil, out, nil
	})
}
