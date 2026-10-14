package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type environmentResource struct{ resourceBase }

type environmentModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Protected types.Bool   `tfsdk:"protected"`
}

// NewEnvironmentResource returns the project environment resource.
func NewEnvironmentResource() resource.Resource { return &environmentResource{} }

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "An environment (for example staging) inside a project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"name":       schema.StringAttribute{Required: true, PlanModifiers: replace},
			"protected": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Require approval for changes to this environment.",
			},
		},
	}
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireEnvironment
	body := map[string]any{"name": plan.Name.ValueString(), "protected": plan.Protected.ValueBool()}
	err := r.client.do(ctx, http.MethodPost, "/api/v1/projects/"+esc(plan.ProjectID.ValueString())+"/environments", body, &out)
	if err != nil {
		resp.Diagnostics.AddError("Create environment failed", err.Error())
		return
	}
	plan.ID = types.StringValue(out.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envs, err := r.client.listEnvironments(ctx, state.ProjectID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read environment failed", err.Error())
		return
	}
	for _, e := range envs {
		if e.ID == state.ID.ValueString() {
			state.Name = types.StringValue(e.Name)
			state.Protected = types.BoolValue(e.Protected)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	err := r.client.do(ctx, http.MethodPatch, "/api/v1/environments/"+esc(state.ID.ValueString()),
		map[string]bool{"protected": plan.Protected.ValueBool()}, nil)
	if err != nil {
		resp.Diagnostics.AddError("Update environment failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/environments/"+esc(state.ID.ValueString()), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete environment failed", err.Error())
	}
}

// ImportState takes project_id/environment_name.
func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	project, name, ok := strings.Cut(req.ID, "/")
	if !ok || project == "" || name == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected project_id/environment_name, got %q", req.ID))
		return
	}
	envs, err := r.client.listEnvironments(ctx, project)
	if err != nil {
		resp.Diagnostics.AddError("Import environment failed", err.Error())
		return
	}
	for _, e := range envs {
		if e.Name == name {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), e.ID)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), project)...)
			return
		}
	}
	resp.Diagnostics.AddError("Environment not found", fmt.Sprintf("project %s has no environment %q", project, name))
}
