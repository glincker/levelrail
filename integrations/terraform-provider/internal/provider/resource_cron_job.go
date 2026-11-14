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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type cronJobResource struct{ resourceBase }

type cronJobModel struct {
	ID                  types.String   `tfsdk:"id"`
	App                 types.String   `tfsdk:"app"`
	Name                types.String   `tfsdk:"name"`
	Schedule            types.String   `tfsdk:"schedule"`
	Timezone            types.String   `tfsdk:"timezone"`
	Command             []types.String `tfsdk:"command"`
	Mode                types.String   `tfsdk:"mode"`
	Enabled             types.Bool     `tfsdk:"enabled"`
	ConcurrencyPolicy   types.String   `tfsdk:"concurrency_policy"`
	CatchUp             types.String   `tfsdk:"catch_up"`
	TimeoutSeconds      types.Int64    `tfsdk:"timeout_seconds"`
	MaxRetries          types.Int64    `tfsdk:"max_retries"`
	RetryBackoffSeconds types.Int64    `tfsdk:"retry_backoff_seconds"`
}

// wireCronJob is the subset of the job API shape this resource manages.
type wireCronJob struct {
	ID                  string   `json:"id,omitempty"`
	Name                string   `json:"name"`
	Mode                string   `json:"mode,omitempty"`
	Command             []string `json:"command"`
	Schedule            string   `json:"schedule"`
	Timezone            string   `json:"timezone,omitempty"`
	Enabled             bool     `json:"enabled"`
	ConcurrencyPolicy   string   `json:"concurrency_policy,omitempty"`
	CatchUp             string   `json:"catch_up,omitempty"`
	TimeoutSeconds      int64    `json:"timeout_seconds,omitempty"`
	MaxRetries          int64    `json:"max_retries,omitempty"`
	RetryBackoffSeconds int64    `json:"retry_backoff_seconds,omitempty"`
}

// NewCronJobResource returns the per-app scheduled job resource.
func NewCronJobResource() resource.Resource { return &cronJobResource{} }

func (r *cronJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cron_job"
}

func (r *cronJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	serverStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, Description: desc}
	}
	zeroInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "A scheduled command of one app, the same job a CronJob document describes.",
		Attributes: map[string]schema.Attribute{
			"id":                    schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true, PlanModifiers: keep},
			"app":                   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, Description: "App name that owns the job."},
			"name":                  schema.StringAttribute{Required: true, Description: "Job name: lowercase letters, digits, hyphens or underscores."},
			"schedule":              schema.StringAttribute{Required: true, Description: "Cron expression, for example 0 3 * * *."},
			"timezone":              serverStr("IANA timezone of the schedule. The server uses UTC when unset."),
			"command":               schema.ListAttribute{Required: true, ElementType: types.StringType, Description: "Command and arguments to run."},
			"mode":                  serverStr("fresh runs a new container per run, exec runs inside the app container."),
			"enabled":               schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether the job is scheduled."},
			"concurrency_policy":    serverStr("What to do when a run is still going: allow, forbid or replace."),
			"catch_up":              serverStr("Missed runs after downtime: skip or run_once."),
			"timeout_seconds":       zeroInt("Seconds before a run is stopped, 0 for the server default."),
			"max_retries":           zeroInt("Retries after a failed run."),
			"retry_backoff_seconds": zeroInt("Seconds between retries."),
		},
	}
}

func (m *cronJobModel) wire() wireCronJob {
	cmd := make([]string, 0, len(m.Command))
	for _, c := range m.Command {
		cmd = append(cmd, c.ValueString())
	}
	return wireCronJob{
		Name: m.Name.ValueString(), Mode: m.Mode.ValueString(), Command: cmd, Schedule: m.Schedule.ValueString(),
		Timezone: m.Timezone.ValueString(), Enabled: m.Enabled.ValueBool(), ConcurrencyPolicy: m.ConcurrencyPolicy.ValueString(),
		CatchUp: m.CatchUp.ValueString(), TimeoutSeconds: m.TimeoutSeconds.ValueInt64(),
		MaxRetries: m.MaxRetries.ValueInt64(), RetryBackoffSeconds: m.RetryBackoffSeconds.ValueInt64(),
	}
}

func (m *cronJobModel) fill(app string, w wireCronJob) {
	m.ID = types.StringValue(app + "/" + w.ID)
	m.App = types.StringValue(app)
	m.Name = types.StringValue(w.Name)
	m.Schedule = types.StringValue(w.Schedule)
	m.Timezone = types.StringValue(w.Timezone)
	m.Command = make([]types.String, 0, len(w.Command))
	for _, c := range w.Command {
		m.Command = append(m.Command, types.StringValue(c))
	}
	m.Mode = types.StringValue(w.Mode)
	m.Enabled = types.BoolValue(w.Enabled)
	m.ConcurrencyPolicy = types.StringValue(w.ConcurrencyPolicy)
	m.CatchUp = types.StringValue(w.CatchUp)
	m.TimeoutSeconds = types.Int64Value(w.TimeoutSeconds)
	m.MaxRetries = types.Int64Value(w.MaxRetries)
	m.RetryBackoffSeconds = types.Int64Value(w.RetryBackoffSeconds)
}

func cronJobPath(app, id string) string {
	return "/api/v1/apps/" + esc(app) + "/jobs/" + esc(id)
}

func (r *cronJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cronJobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireCronJob
	if err := r.client.do(ctx, http.MethodPost, "/api/v1/apps/"+esc(plan.App.ValueString())+"/jobs", plan.wire(), &out); err != nil {
		resp.Diagnostics.AddError("Create cron job failed", err.Error())
		return
	}
	plan.fill(plan.App.ValueString(), out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *cronJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cronJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	app := state.App.ValueString()
	var out wireCronJob
	err := r.client.do(ctx, http.MethodGet, cronJobPath(app, ruleIDOf(state.ID)), nil, &out)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read cron job failed", err.Error())
		return
	}
	state.fill(app, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *cronJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cronJobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireCronJob
	app := plan.App.ValueString()
	if err := r.client.do(ctx, http.MethodPut, cronJobPath(app, ruleIDOf(state.ID)), plan.wire(), &out); err != nil {
		resp.Diagnostics.AddError("Update cron job failed", err.Error())
		return
	}
	plan.fill(app, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *cronJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cronJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, cronJobPath(state.App.ValueString(), ruleIDOf(state.ID)), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete cron job failed", err.Error())
	}
}

// ImportState takes <app>/<job id>.
func (r *cronJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	app, id, ok := strings.Cut(req.ID, "/")
	if !ok || app == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected <app>/<job id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app"), app)...)
}
