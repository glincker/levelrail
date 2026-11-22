package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type dnsRecordResource struct{ resourceBase }

type dnsRecordModel struct {
	ID       types.String `tfsdk:"id"`
	Zone     types.String `tfsdk:"zone"`
	Provider types.String `tfsdk:"dns_provider"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Values   types.Set    `tfsdk:"values"`
}

type wireDNSRecord struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	TTL    int64    `json:"ttl"`
	Values []string `json:"values"`
}

type wireDNSRecordList struct {
	Records []wireDNSRecord `json:"records"`
}

// NewDNSRecordResource returns the DNS record set resource.
func NewDNSRecordResource() resource.Resource { return &dnsRecordResource{} }

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "All values of one DNS record name and type in a zone (a record set).",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Description: "zone/name/type, which is also the import ID.", Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"zone":         schema.StringAttribute{Description: "Zone name the record lives in.", Required: true, PlanModifiers: replace},
			"dns_provider": schema.StringAttribute{Description: "Connected DNS provider holding the zone. Defaults to the only connected provider.", Optional: true, PlanModifiers: replace},
			"name":         schema.StringAttribute{Description: "Record name relative to the zone, or @ for the apex.", Required: true, PlanModifiers: replace},
			"type":         schema.StringAttribute{Description: "Record type such as A, AAAA, CNAME, TXT or MX.", Required: true, PlanModifiers: replace},
			"ttl":          schema.Int64Attribute{Description: "Time to live in seconds. Defaults to the control plane default.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},
			"values":       schema.SetAttribute{Description: "Record values in zone file syntax, except TXT which holds the raw text.", Required: true, ElementType: types.StringType},
		},
	}
}

// relDNSName reduces a name to its form relative to the zone, with @ for the apex.
func relDNSName(name, zone string) string {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	z := strings.TrimSuffix(strings.ToLower(zone), ".")
	switch {
	case n == "" || n == "@" || n == z:
		return "@"
	case strings.HasSuffix(n, "."+z):
		return strings.TrimSuffix(n, "."+z)
	}
	return n
}

func canonValue(v string) string { return strings.TrimSuffix(strings.ToLower(v), ".") }

func sameValues(a, b []string) bool {
	ca, cb := make([]string, len(a)), make([]string, len(b))
	for i, v := range a {
		ca[i] = canonValue(v)
	}
	for i, v := range b {
		cb[i] = canonValue(v)
	}
	slices.Sort(ca)
	slices.Sort(cb)
	return slices.Equal(ca, cb)
}

func (m dnsRecordModel) base() string {
	return "/api/v1/dns/zones/" + esc(m.Zone.ValueString()) + "/records"
}

func (m dnsRecordModel) body(ctx context.Context) (map[string]any, error) {
	var vals []string
	if diags := m.Values.ElementsAs(ctx, &vals, false); diags.HasError() {
		return nil, fmt.Errorf("values: %s", diags.Errors()[0].Detail())
	}
	rec := map[string]any{"name": m.Name.ValueString(), "type": strings.ToUpper(m.Type.ValueString()), "values": vals}
	if !m.TTL.IsNull() && !m.TTL.IsUnknown() {
		rec["ttl"] = m.TTL.ValueInt64()
	}
	return rec, nil
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := plan.body(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Invalid record", err.Error())
		return
	}
	var out struct {
		Record wireDNSRecord `json:"record"`
	}
	if err := r.client.do(ctx, http.MethodPost, plan.base()+dnsProviderQuery(plan.Provider.ValueString()), body, &out); err != nil {
		resp.Diagnostics.AddError("Create DNS record failed", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.Zone.ValueString() + "/" + relDNSName(plan.Name.ValueString(), plan.Zone.ValueString()) + "/" + strings.ToUpper(plan.Type.ValueString()))
	plan.TTL = types.Int64Value(out.Record.TTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	q := url.Values{"type": {strings.ToUpper(state.Type.ValueString())}}
	if p := state.Provider.ValueString(); p != "" {
		q.Set("provider", p)
	}
	var list wireDNSRecordList
	err := r.client.do(ctx, http.MethodGet, state.base()+"?"+q.Encode(), nil, &list)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read DNS record failed", err.Error())
		return
	}
	want := relDNSName(state.Name.ValueString(), state.Zone.ValueString())
	i := slices.IndexFunc(list.Records, func(rec wireDNSRecord) bool {
		return relDNSName(rec.Name, state.Zone.ValueString()) == want
	})
	if i < 0 {
		resp.State.RemoveResource(ctx)
		return
	}
	live := list.Records[i]
	state.TTL = types.Int64Value(live.TTL)
	var have []string
	state.Values.ElementsAs(ctx, &have, false)
	if !sameValues(have, live.Values) {
		state.Values, _ = types.SetValueFrom(ctx, types.StringType, live.Values)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rec, err := plan.body(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Invalid record", err.Error())
		return
	}
	body := map[string]any{
		"original": map[string]string{"name": state.Name.ValueString(), "type": strings.ToUpper(state.Type.ValueString())},
		"record":   rec,
	}
	var out struct {
		Record wireDNSRecord `json:"record"`
	}
	if err := r.client.do(ctx, http.MethodPut, plan.base()+dnsProviderQuery(plan.Provider.ValueString()), body, &out); err != nil {
		resp.Diagnostics.AddError("Update DNS record failed", err.Error())
		return
	}
	plan.ID = state.ID
	plan.TTL = types.Int64Value(out.Record.TTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	q := url.Values{"name": {state.Name.ValueString()}, "type": {strings.ToUpper(state.Type.ValueString())}, "confirm": {state.Name.ValueString()}}
	if p := state.Provider.ValueString(); p != "" {
		q.Set("provider", p)
	}
	err := r.client.do(ctx, http.MethodDelete, state.base()+"?"+q.Encode(), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete DNS record failed", err.Error())
	}
}

// ImportState takes zone/name/type, with @ as the name of the apex.
func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || slices.Contains(parts, "") {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected zone/name/type, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("values"), types.SetValueMust(types.StringType, []attr.Value{}))...)
}
