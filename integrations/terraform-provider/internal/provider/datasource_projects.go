package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type projectsDataSource struct{ dataSourceBase }

type projectsModel struct {
	ID       types.String       `tfsdk:"id"`
	Projects []projectItemModel `tfsdk:"projects"`
}

type projectItemModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type wireProjectItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewProjectsDataSource returns the projects data source.
func NewProjectsDataSource() datasource.DataSource { return &projectsDataSource{} }

func (d *projectsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_projects"
}

func (d *projectsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every project the token can see.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true},
			"projects": schema.ListNestedAttribute{
				Description: "Every project on the control plane.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":   schema.StringAttribute{Computed: true, Description: "Project id."},
					"name": schema.StringAttribute{Computed: true, Description: "Project name."},
				}},
			},
		},
	}
}

func (d *projectsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var projects []wireProjectItem
	if err := d.client.do(ctx, http.MethodGet, "/api/v1/projects", nil, &projects); err != nil {
		resp.Diagnostics.AddError("List projects failed", err.Error())
		return
	}
	state := projectsModel{ID: types.StringValue("projects"), Projects: make([]projectItemModel, 0, len(projects))}
	for _, p := range projects {
		state.Projects = append(state.Projects, projectItemModel{ID: types.StringValue(p.ID), Name: types.StringValue(p.Name)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
