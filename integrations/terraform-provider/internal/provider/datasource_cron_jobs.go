package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type cronJobsDataSource struct{ dataSourceBase }

type cronJobsModel struct {
	ID   types.String       `tfsdk:"id"`
	App  types.String       `tfsdk:"app"`
	Jobs []cronJobItemModel `tfsdk:"jobs"`
}

type cronJobItemModel struct {
	ID         types.String   `tfsdk:"id"`
	Name       types.String   `tfsdk:"name"`
	Schedule   types.String   `tfsdk:"schedule"`
	Timezone   types.String   `tfsdk:"timezone"`
	Mode       types.String   `tfsdk:"mode"`
	Enabled    types.Bool     `tfsdk:"enabled"`
	Command    []types.String `tfsdk:"command"`
	HTTPPath   types.String   `tfsdk:"http_path"`
	HTTPMethod types.String   `tfsdk:"http_method"`
}

// NewCronJobsDataSource returns the per-app scheduled job listing.
func NewCronJobsDataSource() datasource.DataSource { return &cronJobsDataSource{} }

func (d *cronJobsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cron_jobs"
}

func (d *cronJobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every scheduled job of one app, whatever created it. Header values and secret references are not listed.",
		Attributes: map[string]schema.Attribute{
			"id":  schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true},
			"app": schema.StringAttribute{Description: "Name of the app.", Required: true},
			"jobs": schema.ListNestedAttribute{
				Description: "The jobs of the app.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":          schema.StringAttribute{Computed: true},
					"name":        schema.StringAttribute{Computed: true},
					"schedule":    schema.StringAttribute{Computed: true},
					"timezone":    schema.StringAttribute{Computed: true},
					"mode":        schema.StringAttribute{Computed: true},
					"enabled":     schema.BoolAttribute{Computed: true},
					"command":     schema.ListAttribute{Computed: true, ElementType: types.StringType},
					"http_path":   schema.StringAttribute{Computed: true, Description: "Path called by an http job, empty for a command job."},
					"http_method": schema.StringAttribute{Computed: true, Description: "Method of an http job, empty for a command job."},
				}},
			},
		},
	}
}

func (d *cronJobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg cronJobsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var jobs []wireCronJob
	if err := d.client.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(cfg.App.ValueString())+"/jobs", nil, &jobs); err != nil {
		resp.Diagnostics.AddError("List cron jobs failed", err.Error())
		return
	}
	cfg.ID = cfg.App
	cfg.Jobs = make([]cronJobItemModel, 0, len(jobs))
	for _, j := range jobs {
		item := cronJobItemModel{
			ID: types.StringValue(j.ID), Name: types.StringValue(j.Name), Schedule: types.StringValue(j.Schedule),
			Timezone: types.StringValue(j.Timezone), Mode: types.StringValue(j.Mode), Enabled: types.BoolValue(j.Enabled),
			Command: []types.String{}, HTTPPath: types.StringValue(""), HTTPMethod: types.StringValue(""),
		}
		for _, c := range j.Command {
			item.Command = append(item.Command, types.StringValue(c))
		}
		if j.HTTP != nil {
			item.HTTPPath, item.HTTPMethod = types.StringValue(j.HTTP.Path), types.StringValue(j.HTTP.Method)
		}
		cfg.Jobs = append(cfg.Jobs, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
