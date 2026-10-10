package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type migrationPlanInput struct {
	ID string `json:"id,omitempty" jsonschema:"session id, empty lists sessions"`
}

type migrationItemBrief struct {
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Status     string   `json:"status"`
	Selected   bool     `json:"selected"`
	Blocked    bool     `json:"blocked,omitempty"`
	Problems   []string `json:"problems,omitempty"`
	Checked    int      `json:"checked,omitempty"`
	Mismatched int      `json:"mismatched,omitempty"`
}

type migrationPlanOutput struct {
	ID       string               `json:"id,omitempty"`
	Host     string               `json:"host,omitempty"`
	Step     string               `json:"step,omitempty"`
	Verified int                  `json:"verified"`
	Selected int                  `json:"selected"`
	Items    []migrationItemBrief `json:"items,omitempty"`
	Sessions []string             `json:"sessions,omitempty"`
}

// registerMigrationHubTools exposes the server migration plan read-only.
// Starting a copy needs the source password and stays in the dashboard and CLI.
func registerMigrationHubTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_migration_plan",
		Description: "Read-only: a server migration's plan and progress per source database (preflight problems, copy status, verification counts). No id lists session ids. Cannot start a copy.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in migrationPlanInput) (*mcp.CallToolResult, migrationPlanOutput, error) {
		if in.ID == "" {
			list, err := client.ListMigrationSessions(ctx)
			if err != nil {
				return nil, migrationPlanOutput{}, fmt.Errorf("list migration sessions: %w", err)
			}
			out := migrationPlanOutput{}
			for _, s := range list {
				out.Sessions = append(out.Sessions, s.ID)
			}
			return nil, out, nil
		}
		s, err := client.GetMigrationSession(ctx, in.ID)
		if err != nil {
			return nil, migrationPlanOutput{}, fmt.Errorf("get migration plan %q: %w", in.ID, err)
		}
		out := migrationPlanOutput{ID: s.ID, Host: s.Host, Step: s.Step, Verified: s.Summary.Verified, Selected: s.Summary.Selected}
		for _, it := range s.Items {
			b := migrationItemBrief{
				Source: it.SourceDB, Target: it.TargetName, Status: it.Status, Selected: it.Selected,
				Blocked: it.Preflight.Blocked, Checked: it.Checked, Mismatched: it.Mismatched,
			}
			for _, c := range it.Preflight.Checks {
				if c.Severity != "ok" {
					b.Problems = append(b.Problems, c.Severity+": "+c.Message)
				}
			}
			if it.Reason != "" {
				b.Problems = append(b.Problems, it.Reason)
			}
			out.Items = append(out.Items, b)
		}
		return nil, out, nil
	})
}
