package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const templateRevealHint = "Secret values are never returned. Reveal one in the dashboard under the app's environment, or with apps env get <app> <KEY> --reveal (needs secrets:reveal)."

type deployTemplateInput struct {
	ID             string            `json:"id" jsonschema:"template id, from list_service_templates"`
	Name           string            `json:"name,omitempty" jsonschema:"app name; default is the id plus a random suffix"`
	Env            map[string]string `json:"env,omitempty" jsonschema:"template settings KEY to value, see get_service_template"`
	Wait           bool              `json:"wait,omitempty" jsonschema:"block for the outcome"`
	MaxWaitSeconds int               `json:"max_wait_seconds,omitempty" jsonschema:"wait cap in seconds"`
}

type templateServiceOut struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	URL   string `json:"url,omitempty"`
}

type deployTemplateOutput struct {
	App        string               `json:"app"`
	Services   []templateServiceOut `json:"services"`
	SecretKeys []string             `json:"secret_keys,omitempty" jsonschema:"names only, never values"`
	Reveal     string               `json:"reveal,omitempty"`
	Notices    []string             `json:"notices,omitempty"`
	Wait       *waitForDeployOutput `json:"wait,omitempty"`
	WaitNote   string               `json:"wait_note,omitempty"`
}

func newDeployTemplateOutput(res apiclient.ComposeStackDeploy) deployTemplateOutput {
	out := deployTemplateOutput{App: res.AppID, Services: []templateServiceOut{}}
	keys := map[string]struct{}{}
	for _, s := range res.Services {
		svc := templateServiceOut{Name: s.Name, Image: s.Image}
		if len(s.Domains) > 0 {
			svc.URL = "https://" + s.Domains[0]
		}
		out.Services = append(out.Services, svc)
		for _, k := range s.SecretEnv {
			keys[k] = struct{}{}
		}
		for k, v := range s.Env {
			if v == apiclient.MaskedEnvValue {
				keys[k] = struct{}{}
			}
		}
	}
	for k := range keys {
		out.SecretKeys = append(out.SecretKeys, k)
	}
	sort.Strings(out.SecretKeys)
	if len(out.SecretKeys) > 0 {
		out.Reveal = templateRevealHint
	}
	for _, n := range res.Notices {
		out.Notices = append(out.Notices, n.Message)
	}
	return out
}

func registerDeployTemplateTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "deploy_template",
		Description: "Install a catalog template as a new app. Returns app, services, URLs and secret key names only. wait=true blocks for the outcome.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployTemplateInput) (*mcp.CallToolResult, deployTemplateOutput, error) {
		res, err := client.InstallTemplate(ctx, in.ID, apiclient.TemplateInstallRequest{Name: in.Name, Env: in.Env})
		if err != nil {
			return nil, deployTemplateOutput{}, fmt.Errorf("deploy template %q: %w", in.ID, err)
		}
		out := newDeployTemplateOutput(res)
		if in.Wait && out.App != "" {
			w, err := waitDeploy(ctx, client, out.App, "latest", in.MaxWaitSeconds)
			if err != nil {
				return nil, out, fmt.Errorf("deploy template %q started as %q, wait failed: %w", in.ID, out.App, err)
			}
			out.Wait = &w
			out.WaitNote = "waited on the newest deploy of the app"
		}
		return nil, out, nil
	})
}
