package provider

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type dnsZoneResource struct{ resourceBase }

type dnsZoneModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Provider     types.String `tfsdk:"dns_provider"`
	ForceDestroy types.Bool   `tfsdk:"force_destroy"`
	Status       types.String `tfsdk:"status"`
	NameServers  types.List   `tfsdk:"name_servers"`
}

type wireDNSZone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
}

type wireDNSZoneOverview struct {
	Zone wireDNSZone `json:"zone"`
}

// NewDNSZoneResource returns the DNS zone resource.
func NewDNSZoneResource() resource.Resource { return &dnsZoneResource{} }

func (r *dnsZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_zone"
}

func (r *dnsZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A DNS zone at a connected DNS provider. A zone is never adopted by create: one that already exists is refused.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Description: "Zone name, which is also the import ID.", Computed: true, PlanModifiers: keep},
			"name":         schema.StringAttribute{Description: "Zone name such as example.com.", Required: true, PlanModifiers: replace},
			"dns_provider": schema.StringAttribute{Description: "Connected DNS provider to create the zone at. Defaults to the only connected provider.", Optional: true, Computed: true, PlanModifiers: append(keep, stringplanmodifier.RequiresReplace())},
			"force_destroy": schema.BoolAttribute{Description: "Delete the zone even when it still holds records beyond NS and SOA. Defaults to false.", Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"status":       schema.StringAttribute{Description: "Status reported by the provider.", Computed: true},
			"name_servers": schema.ListAttribute{Description: "Name servers to delegate the domain to.", Computed: true, ElementType: types.StringType},
		},
	}
}

func dnsProviderQuery(provider string) string {
	if provider == "" {
		return ""
	}
	return "?provider=" + url.QueryEscape(provider)
}

func (m *dnsZoneModel) apply(ctx context.Context, z wireDNSZone) {
	m.ID, m.Name, m.Status = types.StringValue(z.Name), types.StringValue(z.Name), types.StringValue(z.Status)
	m.Provider = types.StringValue(z.Provider)
	ns := z.NameServers
	if ns == nil {
		ns = []string{}
	}
	m.NameServers, _ = types.ListValueFrom(ctx, types.StringType, ns)
}

func (r *dnsZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var z wireDNSZone
	body := map[string]string{"name": plan.Name.ValueString()}
	if err := r.client.do(ctx, http.MethodPost, "/api/v1/dns/zones"+dnsProviderQuery(plan.Provider.ValueString()), body, &z); err != nil {
		resp.Diagnostics.AddError("Create DNS zone failed", err.Error())
		return
	}
	plan.apply(ctx, z)
	if plan.ForceDestroy.IsNull() || plan.ForceDestroy.IsUnknown() {
		plan.ForceDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var ov wireDNSZoneOverview
	err := r.client.do(ctx, http.MethodGet, "/api/v1/dns/zones/"+esc(state.ID.ValueString())+dnsProviderQuery(state.Provider.ValueString()), nil, &ov)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read DNS zone failed", err.Error())
		return
	}
	state.apply(ctx, ov.Zone)
	if state.ForceDestroy.IsNull() {
		state.ForceDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only force_destroy can change in place, and it lives in state alone.
	state.ForceDestroy = plan.ForceDestroy
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"confirm": state.Name.ValueString(), "force": state.ForceDestroy.ValueBool()}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/dns/zones/"+esc(state.ID.ValueString())+dnsProviderQuery(state.Provider.ValueString()), body, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete DNS zone failed", err.Error())
	}
}

// ImportState takes the zone name.
func (r *dnsZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("force_destroy"), false)...)
}
