package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type databaseResource struct{ resourceBase }

type databaseModel struct {
	Name      types.String `tfsdk:"name"`
	Engine    types.String `tfsdk:"engine"`
	Version   types.String `tfsdk:"version"`
	ProjectID types.String `tfsdk:"project_id"`
}

// NewDatabaseResource returns the managed database resource.
func NewDatabaseResource() resource.Resource { return &databaseResource{} }

func (r *databaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *databaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A managed database. Engine and version cannot change in place because the database holds data.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"engine": schema.StringAttribute{
				Required:      true,
				Description:   "For example postgres or redis.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"version": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *databaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]string{"name": plan.Name.ValueString(), "engine": plan.Engine.ValueString()}
	if v := plan.Version.ValueString(); v != "" {
		body["version"] = v
	}
	if err := r.client.do(ctx, http.MethodPost, "/api/v1/databases", body, nil); err != nil {
		resp.Diagnostics.AddError("Create database failed", err.Error())
		return
	}
	if p := plan.ProjectID.ValueString(); p != "" {
		if err := r.setProject(ctx, plan.Name.ValueString(), p); err != nil {
			resp.Diagnostics.AddError("Assign database to project failed", err.Error())
			return
		}
	}
	var out wireDatabase
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/databases/"+esc(plan.Name.ValueString()), nil, &out); err != nil {
		resp.Diagnostics.AddError("Read database failed", err.Error())
		return
	}
	apply(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseResource) setProject(ctx context.Context, name, project string) error {
	return r.client.do(ctx, http.MethodPut, "/api/v1/databases/"+esc(name)+"/project", map[string]string{"project_id": project}, nil)
}

func (r *databaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireDatabase
	err := r.client.do(ctx, http.MethodGet, "/api/v1/databases/"+esc(state.Name.ValueString()), nil, &out)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read database failed", err.Error())
		return
	}
	apply(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func apply(m *databaseModel, w wireDatabase) {
	m.Engine = types.StringValue(w.Engine)
	m.Version = types.StringValue(w.Version)
	m.ProjectID = types.StringValue(w.ProjectID)
}

func (r *databaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if p := plan.ProjectID.ValueString(); p != "" {
		if err := r.setProject(ctx, plan.Name.ValueString(), p); err != nil {
			resp.Diagnostics.AddError("Assign database to project failed", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/databases/"+esc(state.Name.ValueString()), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete database failed", err.Error())
	}
}

func (r *databaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
