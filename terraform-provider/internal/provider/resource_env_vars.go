package provider

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type envVarsResource struct{ resourceBase }

type envVarsModel struct {
	ID        types.String `tfsdk:"id"`
	Scope     types.String `tfsdk:"scope"`
	ScopeID   types.String `tfsdk:"scope_id"`
	Variables types.Map    `tfsdk:"variables"`
}

// NewEnvVarsResource returns the shared environment variable set resource.
func NewEnvVarsResource() resource.Resource { return &envVarsResource{} }

func (r *envVarsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment_variables"
}

func (r *envVarsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Variables shared by every app of a project or an environment. Only the keys listed here are managed.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"scope":    schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "project or environment."},
			"scope_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "ID of the project or environment."},
			"variables": schema.MapAttribute{
				Required:    true,
				Sensitive:   true,
				ElementType: types.StringType,
			},
		},
	}
}

func mapOf(v types.Map) map[string]string {
	out := map[string]string{}
	for k, e := range v.Elements() {
		if s, ok := e.(types.String); ok {
			out[k] = s.ValueString()
		}
	}
	return out
}

func (r *envVarsResource) liveEnv(ctx context.Context, path string) (map[string]string, error) {
	live := map[string]string{}
	if err := r.client.do(ctx, http.MethodGet, path, nil, &live); err != nil {
		return nil, err
	}
	if live == nil {
		live = map[string]string{}
	}
	return live, nil
}

func (r *envVarsResource) write(ctx context.Context, m *envVarsModel, dropKeys []string) error {
	p, err := envPath(m.Scope.ValueString(), m.ScopeID.ValueString())
	if err != nil {
		return err
	}
	live, err := r.liveEnv(ctx, p)
	if err != nil {
		return err
	}
	for _, k := range dropKeys {
		delete(live, k)
	}
	maps.Copy(live, mapOf(m.Variables))
	return r.client.do(ctx, http.MethodPut, p, live, nil)
}

func (r *envVarsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan envVarsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan, nil); err != nil {
		resp.Diagnostics.AddError("Set environment variables failed", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.Scope.ValueString() + "/" + plan.ScopeID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *envVarsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state envVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := envPath(state.Scope.ValueString(), state.ScopeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read environment variables failed", err.Error())
		return
	}
	live, err := r.liveEnv(ctx, p)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read environment variables failed", err.Error())
		return
	}
	managed := mapOf(state.Variables)
	out := map[string]attr.Value{}
	for k, v := range live {
		// An import has no managed keys yet, so adopt everything.
		if _, ok := managed[k]; ok || state.Variables.IsNull() {
			out[k] = types.StringValue(v)
		}
	}
	state.Variables = types.MapValueMust(types.StringType, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *envVarsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state envVarsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var drop []string
	want := mapOf(plan.Variables)
	for k := range mapOf(state.Variables) {
		if _, ok := want[k]; !ok {
			drop = append(drop, k)
		}
	}
	if err := r.write(ctx, &plan, drop); err != nil {
		resp.Diagnostics.AddError("Set environment variables failed", err.Error())
		return
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *envVarsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state envVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := envPath(state.Scope.ValueString(), state.ScopeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Delete environment variables failed", err.Error())
		return
	}
	live, err := r.liveEnv(ctx, p)
	if isNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Delete environment variables failed", err.Error())
		return
	}
	for k := range mapOf(state.Variables) {
		delete(live, k)
	}
	if err := r.client.do(ctx, http.MethodPut, p, live, nil); err != nil {
		resp.Diagnostics.AddError("Delete environment variables failed", err.Error())
	}
}

// ImportState takes scope/scope_id, for example project/<project id>.
func (r *envVarsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scope, id, ok := strings.Cut(req.ID, "/")
	if _, err := envPath(scope, id); !ok || err != nil || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected project/<id> or environment/<id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), scope)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope_id"), id)...)
}
