package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type iacFileInput struct {
	Name    string `json:"name" jsonschema:"file name used in error messages, e.g. infra/web.yaml"`
	Content string `json:"content" jsonschema:"the YAML text: one or more resource documents separated by ---"`
}

type planApplyInput struct {
	Files           []iacFileInput `json:"files" jsonschema:"resource files. Kinds: Project, Environment, Tag, Database, App, Domain, LoadBalancer, Pipeline, AlertRule. Documents never hold secret values, reference secrets with secretRef"`
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
		Description: "Dry run of declarative resource files: validates them and returns the plan (create, update with a field diff, delete, no change) against live state. Env values are hidden. Line numbered validation errors come back as an error. Changes nothing; run it before apply_resources.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in planApplyInput) (*mcp.CallToolResult, iac.Plan, error) {
		plan, err := client.PlanIaC(ctx, in.request())
		if err != nil {
			return nil, iac.Plan{}, fmt.Errorf("plan resource files: %w", err)
		}
		return nil, plan, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "apply_resources",
		Description: "Apply declarative resource files through the normal API with the caller's own permissions, in dependency order. Pass the expected_plan_hash from plan_apply so it only proceeds if nothing changed since. Each item reports applied, failed, denied or skipped. prune deletes apps the source created that are no longer declared. Secret values cannot be passed here; documents reference secrets by name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in planApplyInput) (*mcp.CallToolResult, iac.ApplyResult, error) {
		res, err := client.ApplyIaC(ctx, in.request())
		if err != nil {
			return nil, iac.ApplyResult{}, fmt.Errorf("apply resource files: %w", err)
		}
		return nil, res, nil
	})
}
