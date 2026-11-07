package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type appSecretResource struct{ resourceBase }

type appSecretModel struct {
	ID    types.String `tfsdk:"id"`
	App   types.String `tfsdk:"app"`
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

// wireSecretKey is the metadata the API returns for a stored secret; it never carries a value.
type wireSecretKey struct {
	Key       string `json:"key"`
	Locked    bool   `json:"locked"`
	BuildTime bool   `json:"build_time"`
	UpdatedAt string `json:"updated_at"`
	Stale     bool   `json:"stale"`
}

type wireSecretValue struct {
	Value string `json:"value"`
}

// NewAppSecretResource returns the app secret resource.
func NewAppSecretResource() resource.Resource { return &appSecretResource{} }

func (r *appSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_secret"
}

func (r *appSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A secret value stored for one app. The API never returns values, so drift in the value itself cannot be detected; only a missing key is.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"app":   schema.StringAttribute{Required: true, PlanModifiers: replace},
			"key":   schema.StringAttribute{Required: true, PlanModifiers: replace},
			"value": schema.StringAttribute{Required: true, Sensitive: true},
		},
	}
}

func secretPath(app, key string) string {
	return "/api/v1/apps/" + esc(app) + "/secrets/" + esc(key)
}

func (r *appSecretResource) put(ctx context.Context, m *appSecretModel) error {
	return r.client.do(ctx, http.MethodPut, secretPath(m.App.ValueString(), m.Key.ValueString()), wireSecretValue{Value: m.Value.ValueString()}, nil)
}

func (r *appSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan appSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Set secret failed", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.App.ValueString() + "/" + plan.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *appSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var keys []wireSecretKey
	err := r.client.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(state.App.ValueString())+"/secrets", nil, &keys)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read secret failed", err.Error())
		return
	}
	for _, k := range keys {
		if k.Key == state.Key.ValueString() {
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *appSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan appSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Set secret failed", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.App.ValueString() + "/" + plan.Key.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *appSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state appSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, secretPath(state.App.ValueString(), state.Key.ValueString()), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete secret failed", err.Error())
	}
}

// ImportState takes <app>/<key>; the value must come from config since the API never reveals it.
func (r *appSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	app, key, ok := strings.Cut(req.ID, "/")
	if !ok || app == "" || key == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected <app>/<key>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), app)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), key)...)
}
