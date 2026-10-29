package mcptools

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stackNameInput struct {
	Name string `json:"name" jsonschema:"stack name"`
}

type stackRunInput struct {
	Name             string            `json:"name" jsonschema:"stack name"`
	Files            []iacFileInput    `json:"files" jsonschema:"resource YAML files"`
	Vars             map[string]string `json:"vars,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty" jsonschema:"from plan_stack"`
}

type applyStackInput struct {
	Name             string            `json:"name" jsonschema:"stack name"`
	Files            []iacFileInput    `json:"files" jsonschema:"resource YAML files"`
	Vars             map[string]string `json:"vars,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty" jsonschema:"from plan_stack"`
	Confirm          bool              `json:"confirm" jsonschema:"must be true; plan_stack first"`
}

type listStacksOutput struct {
	Stacks []apiclient.StackInfo `json:"stacks"`
}

type getStackOutput struct {
	Stack     apiclient.StackInfo      `json:"stack"`
	LastApply *apiclient.StackRevision `json:"last_apply,omitempty"`
}

type stackChangeOut struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// stackRunOutput carries counts and names only: field values, and with them
// any secret, never leave the control plane through this tool.
type stackRunOutput struct {
	Stack           string           `json:"stack"`
	Summary         iac.Summary      `json:"summary"`
	PlanHash        string           `json:"plan_hash,omitempty"`
	Changes         []stackChangeOut `json:"changes,omitempty"`
	Revision        int              `json:"revision,omitempty"`
	Status          string           `json:"status,omitempty"`
	Applied         int              `json:"applied,omitempty"`
	Failed          int              `json:"failed,omitempty"`
	Noop            bool             `json:"noop,omitempty"`
	Replayed        bool             `json:"replayed,omitempty"`
	PendingApproval bool             `json:"pending_approval,omitempty"`
	Issues          []string         `json:"issues,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type planStackOutput struct {
	Stack    string           `json:"stack"`
	Summary  iac.Summary      `json:"summary"`
	PlanHash string           `json:"plan_hash,omitempty"`
	Changes  []stackChangeOut `json:"changes,omitempty"`
	Issues   []string         `json:"issues,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

func stackRequest(in stackRunInput) apiclient.StackRequest {
	files := make([]apiclient.IaCFile, len(in.Files))
	for i, f := range in.Files {
		files[i] = apiclient.IaCFile{Name: f.Name, Content: f.Content}
	}
	return apiclient.StackRequest{Files: files, Vars: in.Vars, ExpectedPlanHash: in.ExpectedPlanHash}
}

func stackChanges(p iac.Plan) []stackChangeOut {
	var out []stackChangeOut
	for _, c := range p.Changes {
		if c.Action == iac.ActionNoop {
			continue
		}
		out = append(out, stackChangeOut{Kind: string(c.Kind), Name: c.Name, Action: string(c.Action), Reason: c.Reason})
	}
	return out
}

func issueStrings(issues []iac.Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.String())
	}
	return out
}

func registerStackTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_stacks",
		Description: "List stacks with latest revision, lock and drift status.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listStacksOutput, error) {
		stacks, _, err := client.ListStacks(ctx)
		if err != nil {
			return nil, listStacksOutput{}, fmt.Errorf("list stacks: %w", err)
		}
		return nil, listStacksOutput{Stacks: stacks}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_stack",
		Description: "One stack: resources, lock, drift and its last apply.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in stackNameInput) (*mcp.CallToolResult, getStackOutput, error) {
		st, _, err := client.GetStack(ctx, in.Name)
		if err != nil {
			return nil, getStackOutput{}, fmt.Errorf("get stack %q: %w", in.Name, err)
		}
		out := getStackOutput{Stack: st}
		if h, _, err := client.StackHistory(ctx, in.Name, 1); err == nil && len(h.Revisions) > 0 {
			out.LastApply = &h.Revisions[0]
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "plan_stack",
		Description: "Dry run of a stack apply: change list and plan hash. Changes nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in stackRunInput) (*mcp.CallToolResult, planStackOutput, error) {
		res, _, err := client.PlanStack(ctx, in.Name, stackRequest(in))
		if err != nil {
			return nil, planStackOutput{}, fmt.Errorf("plan stack %q: %w", in.Name, err)
		}
		return nil, planStackOutput{
			Stack: res.Stack, Summary: res.Plan.Summary, PlanHash: res.Plan.Hash, Changes: stackChanges(res.Plan),
			Issues: issueStrings(res.Issues), Warnings: res.Warnings,
		}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "apply_stack",
		Description: "Apply files to a stack. Needs confirm=true and write access; returns a summary, never secret values.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in applyStackInput) (*mcp.CallToolResult, stackRunOutput, error) {
		if !in.Confirm {
			return nil, stackRunOutput{}, errors.New("apply_stack needs confirm=true; run plan_stack first and review the changes")
		}
		res, _, err := client.ApplyStack(ctx, in.Name, stackRequest(stackRunInput{Name: in.Name, Files: in.Files, Vars: in.Vars, ExpectedPlanHash: in.ExpectedPlanHash}))
		if err != nil {
			return nil, stackRunOutput{}, fmt.Errorf("apply stack %q: %w", in.Name, err)
		}
		out := stackRunOutput{
			Stack: res.Stack, Noop: res.Noop, Replayed: res.Replayed, PendingApproval: res.PendingApproval,
			Issues: issueStrings(res.Issues), Warnings: res.Warnings,
		}
		if res.Revision != nil {
			out.Revision, out.Status = res.Revision.Number, res.Revision.Status
		}
		plan := res.Plan
		if res.Result != nil {
			plan = &res.Result.Plan
			out.Applied, out.Failed = res.Result.Applied, res.Result.Failed
		}
		if plan != nil {
			out.Summary, out.PlanHash, out.Changes = plan.Summary, plan.Hash, stackChanges(*plan)
		}
		return nil, out, nil
	})
}
