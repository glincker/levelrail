package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type templatesDataSource struct{ dataSourceBase }

type templatesModel struct {
	ID        types.String        `tfsdk:"id"`
	Category  types.String        `tfsdk:"category"`
	Templates []templateItemModel `tfsdk:"templates"`
}

type templateItemModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Slogan                types.String `tfsdk:"slogan"`
	Category              types.String `tfsdk:"category"`
	Installable           types.Bool   `tfsdk:"installable"`
	RequiresConfiguration types.Bool   `tfsdk:"requires_configuration"`
	Verified              types.Bool   `tfsdk:"verified"`
}

type wireTemplate struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Slogan                string `json:"slogan"`
	Category              string `json:"category"`
	Installable           bool   `json:"installable"`
	RequiresConfiguration bool   `json:"requires_configuration"`
	Verified              bool   `json:"verified"`
}

// NewTemplatesDataSource returns the service template catalog listing.
func NewTemplatesDataSource() datasource.DataSource { return &templatesDataSource{} }

func (d *templatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_templates"
}

func (d *templatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The service template catalog, optionally narrowed to one category.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true},
			"category": schema.StringAttribute{Description: "Keep only templates of this category.", Optional: true},
			"templates": schema.ListNestedAttribute{
				Description: "The templates of the catalog.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":                     schema.StringAttribute{Computed: true, Description: "Template id."},
					"name":                   schema.StringAttribute{Computed: true, Description: "Display name."},
					"slogan":                 schema.StringAttribute{Computed: true, Description: "One line description."},
					"category":               schema.StringAttribute{Computed: true, Description: "Catalog category."},
					"installable":            schema.BoolAttribute{Computed: true, Description: "True when the template can be installed with one command."},
					"requires_configuration": schema.BoolAttribute{Computed: true, Description: "True when values must be supplied before install."},
					"verified":               schema.BoolAttribute{Computed: true, Description: "True when the template passed the last live smoke run."},
				}},
			},
		},
	}
}

func (d *templatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg templatesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var tpls []wireTemplate
	if err := d.client.do(ctx, http.MethodGet, "/api/v1/service-templates", nil, &tpls); err != nil {
		resp.Diagnostics.AddError("List templates failed", err.Error())
		return
	}
	cfg.ID = types.StringValue("templates")
	cfg.Templates = make([]templateItemModel, 0, len(tpls))
	// The catalog route has no category filter, so narrow here.
	for _, t := range tpls {
		if c := cfg.Category.ValueString(); c != "" && t.Category != c {
			continue
		}
		cfg.Templates = append(cfg.Templates, templateItemModel{
			ID: types.StringValue(t.ID), Name: types.StringValue(t.Name), Slogan: types.StringValue(t.Slogan),
			Category: types.StringValue(t.Category), Installable: types.BoolValue(t.Installable),
			RequiresConfiguration: types.BoolValue(t.RequiresConfiguration), Verified: types.BoolValue(t.Verified),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
