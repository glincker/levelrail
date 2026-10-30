// Package provider implements the Terraform and OpenTofu provider.
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type controlPlaneProvider struct{ version string }

type providerModel struct {
	URL   types.String `tfsdk:"url"`
	Token types.String `tfsdk:"token"`
}

// New returns the provider factory used by main and the tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &controlPlaneProvider{version: version} }
}

func (p *controlPlaneProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = TypeName
	resp.Version = p.version
}

func (p *controlPlaneProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage projects, environments, apps, domains, databases and environment variables on a self-hosted control plane.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the control plane. Falls back to the " + EnvURL + " environment variable.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token. Falls back to the " + EnvToken + " environment variable.",
			},
		},
	}
}

func (p *controlPlaneProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.URL.IsUnknown() || cfg.Token.IsUnknown() {
		resp.Diagnostics.AddError("Unknown provider configuration", "url and token must be known at plan time.")
		return
	}
	url := os.Getenv(EnvURL)
	if !cfg.URL.IsNull() {
		url = cfg.URL.ValueString()
	}
	token := os.Getenv(EnvToken)
	if !cfg.Token.IsNull() {
		token = cfg.Token.ValueString()
	}
	if url == "" {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "Missing url",
			fmt.Sprintf("Set url in the provider block or the %s environment variable.", EnvURL))
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Missing token",
			fmt.Sprintf("Set token in the provider block or the %s environment variable.", EnvToken))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	c := NewClient(url, token)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *controlPlaneProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewEnvironmentResource,
		NewAppResource,
		NewDomainResource,
		NewDatabaseResource,
		NewEnvVarsResource,
		NewStatusPageResource,
		NewStatusComponentResource,
	}
}

func (p *controlPlaneProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewAppsDataSource, NewNodesDataSource}
}

// clientFrom extracts the configured client; nil before Configure ran.
func clientFrom(data any, diags *diag.Diagnostics) *Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *Client, got %T", data))
		return nil
	}
	return c
}
