package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type domainResource struct{ resourceBase }

type domainModel struct {
	ID   types.String `tfsdk:"id"`
	App  types.String `tfsdk:"app"`
	Host types.String `tfsdk:"host"`
}

// NewDomainResource returns the app domain resource.
func NewDomainResource() resource.Resource { return &domainResource{} }

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "One hostname routed to an app.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"app":  schema.StringAttribute{Required: true, PlanModifiers: replace},
			"host": schema.StringAttribute{Required: true, PlanModifiers: replace},
		},
	}
}

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	app, host := plan.App.ValueString(), plan.Host.ValueString()
	live, raw, err := r.client.getApp(ctx, app)
	if err != nil {
		resp.Diagnostics.AddError("Read app failed", err.Error())
		return
	}
	if !slices.Contains(live.Domains, host) {
		raw["domains"] = append(live.Domains, host)
		if err := r.client.putApp(ctx, app, raw); err != nil {
			resp.Diagnostics.AddError("Add domain failed", err.Error())
			return
		}
	}
	plan.ID = types.StringValue(app + "/" + host)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	live, _, err := r.client.getApp(ctx, state.App.ValueString())
	if isNotFound(err) || (err == nil && !slices.Contains(live.Domains, state.Host.ValueString())) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read domain failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	app, host := state.App.ValueString(), state.Host.ValueString()
	live, raw, err := r.client.getApp(ctx, app)
	if isNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read app failed", err.Error())
		return
	}
	kept := slices.DeleteFunc(slices.Clone(live.Domains), func(d string) bool { return d == host })
	if len(kept) == len(live.Domains) {
		return
	}
	raw["domains"] = kept
	if err := r.client.putApp(ctx, app, raw); err != nil {
		resp.Diagnostics.AddError("Remove domain failed", err.Error())
	}
}

// ImportState takes app_name/host.
func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	app, host, ok := strings.Cut(req.ID, "/")
	if !ok || app == "" || host == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected app_name/host, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), app)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host"), host)...)
}
