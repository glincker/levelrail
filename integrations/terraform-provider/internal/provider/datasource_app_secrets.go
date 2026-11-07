package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type appSecretsDataSource struct{ dataSourceBase }

type appSecretsModel struct {
	ID      types.String         `tfsdk:"id"`
	App     types.String         `tfsdk:"app"`
	Secrets []appSecretMetaModel `tfsdk:"secrets"`
}

type appSecretMetaModel struct {
	Key       types.String `tfsdk:"key"`
	Locked    types.Bool   `tfsdk:"locked"`
	BuildTime types.Bool   `tfsdk:"build_time"`
	UpdatedAt types.String `tfsdk:"updated_at"`
	Stale     types.Bool   `tfsdk:"stale"`
}

// NewAppSecretsDataSource returns the secret metadata data source.
func NewAppSecretsDataSource() datasource.DataSource { return &appSecretsDataSource{} }

func (d *appSecretsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_secrets"
}

func (d *appSecretsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Secret key metadata of one app (names, lock and age). Values are never exposed.",
		Attributes: map[string]schema.Attribute{
			"id":  schema.StringAttribute{Computed: true},
			"app": schema.StringAttribute{Required: true},
			"secrets": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key":        schema.StringAttribute{Computed: true},
					"locked":     schema.BoolAttribute{Computed: true},
					"build_time": schema.BoolAttribute{Computed: true},
					"updated_at": schema.StringAttribute{Computed: true},
					"stale":      schema.BoolAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *appSecretsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg appSecretsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var keys []wireSecretKey
	if err := d.client.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(cfg.App.ValueString())+"/secrets", nil, &keys); err != nil {
		resp.Diagnostics.AddError("List secrets failed", err.Error())
		return
	}
	cfg.ID = cfg.App
	cfg.Secrets = make([]appSecretMetaModel, 0, len(keys))
	for _, k := range keys {
		cfg.Secrets = append(cfg.Secrets, appSecretMetaModel{
			Key: types.StringValue(k.Key), Locked: types.BoolValue(k.Locked), BuildTime: types.BoolValue(k.BuildTime),
			UpdatedAt: types.StringValue(k.UpdatedAt), Stale: types.BoolValue(k.Stale),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
