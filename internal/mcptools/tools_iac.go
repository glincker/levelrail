package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type iacFileInput struct {
	Name    string `json:"name" jsonschema:"file name, e.g. infra/web.yaml"`
	Content string `json:"content" jsonschema:"YAML, documents separated by ---"`
}

type planApplyInput struct {
	Files           []iacFileInput `json:"files" jsonschema:"resource YAML files (Project, Environment, Tag, Database, App, Domain, LoadBalancer, Pipeline, AlertRule). Secrets only by secretRef"`
	Source          string         `json:"source,omitempty" jsonschema:"name of this apply source; apps it creates carry a managed-by tag. Required for prune"`
	Project         string         `json:"project,omitempty" jsonschema:"only the documents belonging to this project"`
	Prune           bool           `json:"prune,omitempty" jsonschema:"delete apps this source created that the files no longer declare"`
	NoDeploy        bool           `json:"no_deploy,omitempty" jsonschema:"do not restart running apps to apply env changes"`
	ContinueOnErr   bool           `json:"continue_on_error,omitempty" jsonschema:"keep applying after an item fails"`
	ExpectedPlanSHA string         `json:"expected_plan_hash,omitempty" jsonschema:"the hash from plan_apply; apply only proceeds if live state still matches it"`
}

func (in planApplyInput) request() apiclient.IaCRequest {
	files := make([]apiclient.IaCFile, len(in.Files))
	for i, f := range in.Files {
		files[i] = apiclient.IaCFile{Name: f.Name, Content: f.Content}
	}
	return apiclient.IaCRequest{Files: files, Source: in.Source, Project: in.Project, Prune: in.Prune, NoDeploy: in.NoDeploy,
		ContinueOnError: in.ContinueOnErr, ExpectedPlanHash: in.ExpectedPlanSHA}
}

func registerIaCTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "plan_apply",
		Description: "Dry run of resource files: validation and a plan (create, update diff, delete) against live state. Env values hidden. Changes nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in planApplyInput) (*mcp.CallToolResult, iac.Plan, error) {
		plan, err := client.PlanIaC(ctx, in.request())
		if err != nil {
			return nil, iac.Plan{}, fmt.Errorf("plan resource files: %w", err)
		}
		return nil, plan, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "apply_resources",
		Description: "Apply resource files in dependency order with the caller's permissions. Pass expected_plan_hash from plan_apply to refuse drift. Items report applied, failed, denied or skipped. prune deletes undeclared apps.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in planApplyInput) (*mcp.CallToolResult, iac.ApplyResult, error) {
		res, err := client.ApplyIaC(ctx, in.request())
		if err != nil {
			return nil, iac.ApplyResult{}, fmt.Errorf("apply resource files: %w", err)
		}
		return nil, res, nil
	})
}
