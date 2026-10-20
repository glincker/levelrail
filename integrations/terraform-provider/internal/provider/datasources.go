package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type dataSourceBase struct{ client *Client }

func (b *dataSourceBase) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	b.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

type appsDataSource struct{ dataSourceBase }

type appsModel struct {
	ID   types.String   `tfsdk:"id"`
	Apps []appItemModel `tfsdk:"apps"`
}

type appItemModel struct {
	Name      types.String   `tfsdk:"name"`
	Image     types.String   `tfsdk:"image"`
	Port      types.Int64    `tfsdk:"port"`
	ProjectID types.String   `tfsdk:"project_id"`
	Domains   []types.String `tfsdk:"domains"`
}

// NewAppsDataSource returns the apps data source.
func NewAppsDataSource() datasource.DataSource { return &appsDataSource{} }

func (d *appsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apps"
}

func (d *appsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every app the token can see.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"apps": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name":       schema.StringAttribute{Computed: true},
					"image":      schema.StringAttribute{Computed: true},
					"port":       schema.Int64Attribute{Computed: true},
					"project_id": schema.StringAttribute{Computed: true},
					"domains":    schema.ListAttribute{Computed: true, ElementType: types.StringType},
				}},
			},
		},
	}
}

func (d *appsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var apps []wireApp
	if err := d.client.do(ctx, http.MethodGet, "/api/v1/apps", nil, &apps); err != nil {
		resp.Diagnostics.AddError("List apps failed", err.Error())
		return
	}
	state := appsModel{ID: types.StringValue("apps"), Apps: make([]appItemModel, 0, len(apps))}
	for _, a := range apps {
		item := appItemModel{
			Name:      types.StringValue(a.Name),
			Image:     types.StringValue(a.Image),
			Port:      types.Int64Value(a.Port),
			ProjectID: types.StringValue(a.ProjectID),
			Domains:   make([]types.String, 0, len(a.Domains)),
		}
		for _, dom := range a.Domains {
			item.Domains = append(item.Domains, types.StringValue(dom))
		}
		state.Apps = append(state.Apps, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

type nodesDataSource struct{ dataSourceBase }

type nodesModel struct {
	ID    types.String    `tfsdk:"id"`
	Nodes []nodeItemModel `tfsdk:"nodes"`
}

type nodeItemModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Address     types.String `tfsdk:"address"`
	Status      types.String `tfsdk:"status"`
	Region      types.String `tfsdk:"region"`
	Schedulable types.Bool   `tfsdk:"schedulable"`
}

// NewNodesDataSource returns the nodes data source.
func NewNodesDataSource() datasource.DataSource { return &nodesDataSource{} }

func (d *nodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nodes"
}

func (d *nodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every node of the fleet. Listing nodes needs a root token.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"nodes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":          schema.StringAttribute{Computed: true},
					"name":        schema.StringAttribute{Computed: true},
					"address":     schema.StringAttribute{Computed: true},
					"status":      schema.StringAttribute{Computed: true},
					"region":      schema.StringAttribute{Computed: true},
					"schedulable": schema.BoolAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *nodesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var nodes []wireNode
	if err := d.client.do(ctx, http.MethodGet, "/api/v1/nodes", nil, &nodes); err != nil {
		resp.Diagnostics.AddError("List nodes failed", err.Error())
		return
	}
	state := nodesModel{ID: types.StringValue("nodes"), Nodes: make([]nodeItemModel, 0, len(nodes))}
	for _, n := range nodes {
		state.Nodes = append(state.Nodes, nodeItemModel{
			ID:          types.StringValue(n.ID),
			Name:        types.StringValue(n.Name),
			Address:     types.StringValue(n.Address),
			Status:      types.StringValue(n.Status),
			Region:      types.StringValue(n.Region),
			Schedulable: types.BoolValue(n.Schedulable),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
