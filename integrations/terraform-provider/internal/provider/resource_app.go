package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type appResource struct{ resourceBase }

type appModel struct {
	Name        types.String `tfsdk:"name"`
	Image       types.String `tfsdk:"image"`
	Port        types.Int64  `tfsdk:"port"`
	Replicas    types.Int64  `tfsdk:"replicas"`
	Strategy    types.String `tfsdk:"strategy"`
	BindAddress types.String `tfsdk:"bind_address"`
	ProjectID   types.String `tfsdk:"project_id"`
	Env         types.Map    `tfsdk:"env"`
}

// NewAppResource returns the app resource.
func NewAppResource() resource.Resource { return &appResource{} }

func (r *appResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (r *appResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An app deployed from a container image. Domains are managed with the domain resource, not here.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"image": schema.StringAttribute{Required: true},
			"port": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"replicas": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"strategy":     schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep},
			"bind_address": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep},
			"project_id":   schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep},
			"env": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "Plain environment variables. Unset to leave live variables untouched.",
			},
		},
	}
}

// body overlays the planned fields on a base app body (live for updates).
func (m appModel) body(base map[string]any, withName bool) map[string]any {
	body := base
	if body == nil {
		body = map[string]any{}
	}
	if withName {
		body["name"] = m.Name.ValueString()
	}
	body["image"] = m.Image.ValueString()
	if !m.Port.IsNull() && !m.Port.IsUnknown() {
		body["port"] = m.Port.ValueInt64()
	}
	if !m.Replicas.IsNull() && !m.Replicas.IsUnknown() {
		body["replicas"] = m.Replicas.ValueInt64()
	}
	if s := m.Strategy.ValueString(); s != "" {
		body["strategy"] = s
	}
	if s := m.BindAddress.ValueString(); s != "" {
		body["bind_address"] = s
	}
	if !m.Env.IsNull() && !m.Env.IsUnknown() {
		env := map[string]string{}
		for k, v := range m.Env.Elements() {
			if s, ok := v.(types.String); ok {
				env[k] = s.ValueString()
			}
		}
		body["env"] = env
	}
	return body
}

func (m *appModel) fromWire(w wireApp) {
	m.Image = types.StringValue(w.Image)
	m.Port = types.Int64Value(w.Port)
	m.Replicas = types.Int64Value(w.Replicas)
	m.Strategy = types.StringValue(w.Strategy)
	m.BindAddress = types.StringValue(w.BindAddress)
	m.ProjectID = types.StringValue(w.ProjectID)
	if !m.Env.IsNull() {
		elems := make(map[string]attr.Value, len(w.Env))
		for k, v := range w.Env {
			elems[k] = types.StringValue(v)
		}
		m.Env = types.MapValueMust(types.StringType, elems)
	}
}

func (r *appResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan appModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := plan.body(nil, true)
	if p := plan.ProjectID.ValueString(); p != "" {
		body["project_id"] = p
	}
	if err := r.client.do(ctx, http.MethodPost, "/api/v1/apps", body, nil); err != nil {
		resp.Diagnostics.AddError("Create app failed", err.Error())
		return
	}
	live, _, err := r.client.getApp(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read app failed", err.Error())
		return
	}
	plan.fromWire(live)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *appResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	live, _, err := r.client.getApp(ctx, state.Name.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read app failed", err.Error())
		return
	}
	state.fromWire(live)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *appResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state appModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, raw, err := r.client.getApp(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read app failed", err.Error())
		return
	}
	// PUT replaces the whole app, so start from the live body to keep
	// settings this resource does not manage.
	if err := r.client.putApp(ctx, plan.Name.ValueString(), plan.body(raw, false)); err != nil {
		resp.Diagnostics.AddError("Update app failed", err.Error())
		return
	}
	if p := plan.ProjectID.ValueString(); p != "" && p != state.ProjectID.ValueString() {
		err := r.client.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(plan.Name.ValueString())+"/project", map[string]string{"project_id": p}, nil)
		if err != nil {
			resp.Diagnostics.AddError("Move app to project failed", err.Error())
			return
		}
	}
	live, _, err := r.client.getApp(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read app failed", err.Error())
		return
	}
	plan.fromWire(live)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *appResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state appModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/apps/"+esc(state.Name.ValueString()), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete app failed", err.Error())
	}
}

func (r *appResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
