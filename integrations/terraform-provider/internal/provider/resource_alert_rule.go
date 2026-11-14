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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type alertRuleResource struct{ resourceBase }

type alertRuleModel struct {
	ID                    types.String  `tfsdk:"id"`
	App                   types.String  `tfsdk:"app"`
	Name                  types.String  `tfsdk:"name"`
	Kind                  types.String  `tfsdk:"kind"`
	Metric                types.String  `tfsdk:"metric"`
	Comparator            types.String  `tfsdk:"comparator"`
	Threshold             types.Float64 `tfsdk:"threshold"`
	ForDuration           types.String  `tfsdk:"for_duration"`
	RestartCountThreshold types.Int64   `tfsdk:"restart_count_threshold"`
	RestartWindow         types.String  `tfsdk:"restart_window"`
	ChannelID             types.String  `tfsdk:"channel_id"`
	Enabled               types.Bool    `tfsdk:"enabled"`
}

// wireAlertRule is the subset of the alert rule API shape this resource manages.
type wireAlertRule struct {
	ID                    string  `json:"id,omitempty"`
	Name                  string  `json:"name"`
	Kind                  string  `json:"kind"`
	ChannelID             string  `json:"channel_id,omitempty"`
	Metric                string  `json:"metric,omitempty"`
	Comparator            string  `json:"comparator,omitempty"`
	Threshold             float64 `json:"threshold"`
	ForDuration           string  `json:"for_duration,omitempty"`
	RestartCountThreshold int     `json:"restart_count_threshold"`
	RestartWindow         string  `json:"restart_window,omitempty"`
	Enabled               bool    `json:"enabled"`
}

// NewAlertRuleResource returns the per-app alert rule resource.
func NewAlertRuleResource() resource.Resource { return &alertRuleResource{} }

func (r *alertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rule"
}

func (r *alertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	emptyStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "An alert rule on one app, for example a metric threshold or a crash loop.",
		Attributes: map[string]schema.Attribute{
			"id":                      schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"app":                     schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "App name that owns the rule."},
			"name":                    schema.StringAttribute{Description: "Name of the resource.", Required: true},
			"kind":                    schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Rule kind, for example threshold or crashloop."},
			"metric":                  emptyStr("Metric a threshold rule watches."),
			"comparator":              emptyStr("Comparator of a threshold rule."),
			"threshold":               schema.Float64Attribute{Description: "Value that triggers the rule.", Optional: true, Computed: true, Default: float64default.StaticFloat64(0)},
			"for_duration":            emptyStr("How long the condition must hold, as a duration such as 5m."),
			"restart_count_threshold": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Description: "Restarts that trip a crashloop rule."},
			"restart_window":          emptyStr("Window of a crashloop rule, as a duration."),
			"channel_id":              emptyStr("Notification channel to alert."),
			"enabled":                 schema.BoolAttribute{Description: "Whether the rule is active.", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		},
	}
}

func (m *alertRuleModel) wire() wireAlertRule {
	return wireAlertRule{
		Name: m.Name.ValueString(), Kind: m.Kind.ValueString(), ChannelID: m.ChannelID.ValueString(),
		Metric: m.Metric.ValueString(), Comparator: m.Comparator.ValueString(), Threshold: m.Threshold.ValueFloat64(),
		ForDuration: m.ForDuration.ValueString(), RestartCountThreshold: int(m.RestartCountThreshold.ValueInt64()),
		RestartWindow: m.RestartWindow.ValueString(), Enabled: m.Enabled.ValueBool(),
	}
}

func (m *alertRuleModel) fill(app string, w wireAlertRule) {
	m.ID = types.StringValue(app + "/" + w.ID)
	m.App = types.StringValue(app)
	m.Name = types.StringValue(w.Name)
	m.Kind = types.StringValue(w.Kind)
	m.Metric = types.StringValue(w.Metric)
	m.Comparator = types.StringValue(w.Comparator)
	m.Threshold = types.Float64Value(w.Threshold)
	m.ForDuration = types.StringValue(w.ForDuration)
	m.RestartCountThreshold = types.Int64Value(int64(w.RestartCountThreshold))
	m.RestartWindow = types.StringValue(w.RestartWindow)
	m.ChannelID = types.StringValue(w.ChannelID)
	m.Enabled = types.BoolValue(w.Enabled)
}

func alertRulePath(app, id string) string {
	return "/api/v1/apps/" + esc(app) + "/alerts/" + esc(id)
}

func ruleIDOf(state types.String) string {
	_, id, _ := strings.Cut(state.ValueString(), "/")
	return id
}

func (r *alertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireAlertRule
	if err := r.client.do(ctx, http.MethodPost, "/api/v1/apps/"+esc(plan.App.ValueString())+"/alerts", plan.wire(), &out); err != nil {
		resp.Diagnostics.AddError("Create alert rule failed", err.Error())
		return
	}
	plan.fill(plan.App.ValueString(), out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	app, id := state.App.ValueString(), ruleIDOf(state.ID)
	var rules []wireAlertRule
	err := r.client.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(app)+"/alerts", nil, &rules)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read alert rule failed", err.Error())
		return
	}
	for _, w := range rules {
		if w.ID == id {
			state.fill(app, w)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *alertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireAlertRule
	app := plan.App.ValueString()
	if err := r.client.do(ctx, http.MethodPut, alertRulePath(app, ruleIDOf(state.ID)), plan.wire(), &out); err != nil {
		resp.Diagnostics.AddError("Update alert rule failed", err.Error())
		return
	}
	plan.fill(app, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, alertRulePath(state.App.ValueString(), ruleIDOf(state.ID)), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete alert rule failed", err.Error())
	}
}

// ImportState takes <app>/<rule id>.
func (r *alertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	app, id, ok := strings.Cut(req.ID, "/")
	if !ok || app == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected <app>/<rule id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), app)...)
}
