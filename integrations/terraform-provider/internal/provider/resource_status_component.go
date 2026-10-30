package provider

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var statusComponentKinds = []string{"app", "domain", "check"}

// wireStatusComponent mirrors one component of /api/v1/status-page/components.
type wireStatusComponent struct {
	ID          string `json:"id,omitempty"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	DisplayName string `json:"display_name"`
	Position    int64  `json:"position"`
}

type statusComponentResource struct{ resourceBase }

type statusComponentModel struct {
	ID     types.String `tfsdk:"id"`
	Kind   types.String `tfsdk:"kind"`
	Target types.String `tfsdk:"target"`
	Name   types.String `tfsdk:"name"`
	Order  types.Int64  `tfsdk:"order"`
}

// NewStatusComponentResource returns the status page component resource.
func NewStatusComponentResource() resource.Resource { return &statusComponentResource{} }

func (r *statusComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_component"
}

func (r *statusComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "One row on the public status page, backed by an app, a domain or a check.",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"kind":   schema.StringAttribute{Required: true, Description: "One of app, domain or check."},
			"target": schema.StringAttribute{Required: true, Description: "App name, domain host or check ID, depending on kind."},
			"name":   schema.StringAttribute{Required: true, Description: "Label shown on the page."},
			"order":  schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Description: "Sort position, lowest first."},
		},
	}
}

// ValidateConfig rejects unknown kinds at plan time instead of at apply.
func (r *statusComponentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m statusComponentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Kind.IsNull() && !m.Kind.IsUnknown() && !slices.Contains(statusComponentKinds, m.Kind.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("kind"), "Invalid kind",
			fmt.Sprintf("kind must be one of %s, got %q", strings.Join(statusComponentKinds, ", "), m.Kind.ValueString()))
	}
	if !m.Target.IsNull() && !m.Target.IsUnknown() && strings.TrimSpace(m.Target.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("target"), "Invalid target", "target must not be empty")
	}
}

func (m statusComponentModel) wire() wireStatusComponent {
	return wireStatusComponent{
		Kind: m.Kind.ValueString(), Target: m.Target.ValueString(),
		DisplayName: m.Name.ValueString(), Position: m.Order.ValueInt64(),
	}
}

func (m *statusComponentModel) fromWire(w wireStatusComponent) {
	m.ID = types.StringValue(w.ID)
	m.Kind = types.StringValue(w.Kind)
	m.Target = types.StringValue(w.Target)
	m.Name = types.StringValue(w.DisplayName)
	m.Order = types.Int64Value(w.Position)
}

func (r *statusComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireStatusComponent
	if err := r.client.do(ctx, http.MethodPost, "/api/v1/status-page/components", plan.wire(), &out); err != nil {
		resp.Diagnostics.AddError("Create status component failed", err.Error())
		return
	}
	plan.fromWire(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read lists components because the API has no single-item GET.
func (r *statusComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var all []wireStatusComponent
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/status-page/components", nil, &all); err != nil {
		resp.Diagnostics.AddError("Read status component failed", err.Error())
		return
	}
	i := slices.IndexFunc(all, func(c wireStatusComponent) bool { return c.ID == state.ID.ValueString() })
	if i < 0 {
		resp.State.RemoveResource(ctx)
		return
	}
	state.fromWire(all[i])
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *statusComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state statusComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireStatusComponent
	if err := r.client.do(ctx, http.MethodPut, "/api/v1/status-page/components/"+esc(state.ID.ValueString()), plan.wire(), &out); err != nil {
		resp.Diagnostics.AddError("Update status component failed", err.Error())
		return
	}
	plan.fromWire(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *statusComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/status-page/components/"+esc(state.ID.ValueString()), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete status component failed", err.Error())
	}
}

func (r *statusComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
