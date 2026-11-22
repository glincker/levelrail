package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type latestDeployDataSource struct{ dataSourceBase }

type latestDeployModel struct {
	ID         types.String `tfsdk:"id"`
	App        types.String `tfsdk:"app"`
	Status     types.String `tfsdk:"status"`
	DeployID   types.String `tfsdk:"deploy_id"`
	Image      types.String `tfsdk:"image"`
	CommitSHA  types.String `tfsdk:"commit_sha"`
	StartedAt  types.String `tfsdk:"started_at"`
	FinishedAt types.String `tfsdk:"finished_at"`
	Found      types.Bool   `tfsdk:"found"`
}

type wireDeployAttempt struct {
	ID         string  `json:"id"`
	Image      string  `json:"image"`
	CommitSHA  string  `json:"commit_sha"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
}

// NewLatestDeployDataSource returns the newest deploy of one app.
func NewLatestDeployDataSource() datasource.DataSource { return &latestDeployDataSource{} }

func (d *latestDeployDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_latest_deploy"
}

func (d *latestDeployDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The newest deploy of one app, optionally the newest one with a given status.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true},
			"app":         schema.StringAttribute{Description: "Name of the app.", Required: true},
			"status":      schema.StringAttribute{Description: "Keep only deploys with this status, for example succeeded. The status of the deploy found is returned here.", Optional: true, Computed: true},
			"deploy_id":   schema.StringAttribute{Description: "Id of the deploy, empty when the app has none.", Computed: true},
			"image":       schema.StringAttribute{Description: "Image the deploy ran.", Computed: true},
			"commit_sha":  schema.StringAttribute{Description: "Git commit of a source deploy, empty otherwise.", Computed: true},
			"started_at":  schema.StringAttribute{Description: "When the deploy started, RFC 3339.", Computed: true},
			"finished_at": schema.StringAttribute{Description: "When the deploy finished, empty while it runs.", Computed: true},
			"found":       schema.BoolAttribute{Description: "False when the app has no deploy matching the filter.", Computed: true},
		},
	}
}

func (d *latestDeployDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg latestDeployModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	path := "/api/v1/apps/" + esc(cfg.App.ValueString()) + "/deploy-attempts?limit=1"
	if s := cfg.Status.ValueString(); s != "" {
		path += "&status=" + esc(s)
	}
	var attempts []wireDeployAttempt
	if err := d.client.do(ctx, http.MethodGet, path, nil, &attempts); err != nil {
		resp.Diagnostics.AddError("List deploys failed", err.Error())
		return
	}
	cfg.ID = cfg.App
	cfg.Found = types.BoolValue(len(attempts) > 0)
	cfg.DeployID, cfg.Image, cfg.CommitSHA = types.StringValue(""), types.StringValue(""), types.StringValue("")
	cfg.StartedAt, cfg.FinishedAt = types.StringValue(""), types.StringValue("")
	if len(attempts) > 0 {
		a := attempts[0]
		cfg.DeployID, cfg.Image, cfg.CommitSHA = types.StringValue(a.ID), types.StringValue(a.Image), types.StringValue(a.CommitSHA)
		cfg.Status, cfg.StartedAt = types.StringValue(a.Status), types.StringValue(a.StartedAt)
		if a.FinishedAt != nil {
			cfg.FinishedAt = types.StringValue(*a.FinishedAt)
		}
	}
	if cfg.Status.IsNull() || cfg.Status.IsUnknown() {
		cfg.Status = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
