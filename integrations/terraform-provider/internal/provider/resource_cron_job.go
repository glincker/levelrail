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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
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
	HTTP                *cronHTTPModel `tfsdk:"http"`
}

// cronHTTPModel is the request of an http job, sent to a path on the app itself.
type cronHTTPModel struct {
	Path       types.String          `tfsdk:"path"`
	Method     types.String          `tfsdk:"method"`
	Body       types.String          `tfsdk:"body"`
	Background types.Bool            `tfsdk:"background"`
	Headers    []cronHTTPHeaderModel `tfsdk:"headers"`
}

type cronHTTPHeaderModel struct {
	Name      types.String `tfsdk:"name"`
	Value     types.String `tfsdk:"value"`
	SecretRef types.String `tfsdk:"secret_ref"`
	Prefix    types.String `tfsdk:"prefix"`
}

type wireCronHTTP struct {
	Method     string           `json:"method,omitempty"`
	Path       string           `json:"path"`
	Body       string           `json:"body,omitempty"`
	Background bool             `json:"background,omitempty"`
	Headers    []wireCronHeader `json:"headers,omitempty"`
}

type wireCronHeader struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
}

// wireCronJob is the subset of the job API shape this resource manages.
type wireCronJob struct {
	ID                  string        `json:"id,omitempty"`
	Name                string        `json:"name"`
	Mode                string        `json:"mode,omitempty"`
	Command             []string      `json:"command"`
	Schedule            string        `json:"schedule"`
	Timezone            string        `json:"timezone,omitempty"`
	Enabled             bool          `json:"enabled"`
	ConcurrencyPolicy   string        `json:"concurrency_policy,omitempty"`
	CatchUp             string        `json:"catch_up,omitempty"`
	TimeoutSeconds      int64         `json:"timeout_seconds,omitempty"`
	MaxRetries          int64         `json:"max_retries,omitempty"`
	RetryBackoffSeconds int64         `json:"retry_backoff_seconds,omitempty"`
	HTTP                *wireCronHTTP `json:"http,omitempty"`
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
			"id":       schema.StringAttribute{Description: "Identifier assigned by the provider.", Computed: true, PlanModifiers: keep},
			"app":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, Description: "App name that owns the job."},
			"name":     schema.StringAttribute{Required: true, Description: "Job name: lowercase letters, digits, hyphens or underscores."},
			"schedule": schema.StringAttribute{Required: true, Description: "Cron expression, for example 0 3 * * *."},
			"timezone": serverStr("IANA timezone of the schedule. The server uses UTC when unset."),
			"command":  schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Command and arguments to run. Set exactly one of command or http."},
			"mode":     serverStr("fresh runs a new container per run, exec runs inside the app container, http is set by the http block."),
			"http": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Call a path on the app instead of running a command.",
				Attributes: map[string]schema.Attribute{
					"path":       schema.StringAttribute{Required: true, Description: "Path on the app, starting with a single /."},
					"method":     schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("GET"), Description: "GET, POST, PUT, PATCH or DELETE."},
					"body":       schema.StringAttribute{Optional: true, Description: "Request body."},
					"background": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Return at once and let the control plane await the reply."},
					"headers": schema.ListNestedAttribute{
						Optional:    true,
						Description: "Request headers. Set a literal value or a secret_ref naming an app env var, never secret text.",
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"name":       schema.StringAttribute{Required: true},
							"value":      schema.StringAttribute{Optional: true, Description: "Literal header value."},
							"secret_ref": schema.StringAttribute{Optional: true, Description: "Name of the app env var that holds the value."},
							"prefix":     schema.StringAttribute{Optional: true, Description: "Text before the secret, such as Bearer. Only with secret_ref."},
						}},
					},
				},
			},
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
	w := wireCronJob{
		Name: m.Name.ValueString(), Mode: m.Mode.ValueString(), Command: cmd, Schedule: m.Schedule.ValueString(),
		Timezone: m.Timezone.ValueString(), Enabled: m.Enabled.ValueBool(), ConcurrencyPolicy: m.ConcurrencyPolicy.ValueString(),
		CatchUp: m.CatchUp.ValueString(), TimeoutSeconds: m.TimeoutSeconds.ValueInt64(),
		MaxRetries: m.MaxRetries.ValueInt64(), RetryBackoffSeconds: m.RetryBackoffSeconds.ValueInt64(),
	}
	if m.HTTP != nil {
		w.Mode = "http"
		h := &wireCronHTTP{Path: m.HTTP.Path.ValueString(), Method: m.HTTP.Method.ValueString(), Body: m.HTTP.Body.ValueString(), Background: m.HTTP.Background.ValueBool()}
		for _, hd := range m.HTTP.Headers {
			h.Headers = append(h.Headers, wireCronHeader{Name: hd.Name.ValueString(), Value: hd.Value.ValueString(), SecretRef: hd.SecretRef.ValueString(), Prefix: hd.Prefix.ValueString()})
		}
		w.HTTP = h
	}
	return w
}

func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func (m *cronJobModel) fill(app string, w wireCronJob) {
	m.ID = types.StringValue(app + "/" + w.ID)
	m.App = types.StringValue(app)
	m.Name = types.StringValue(w.Name)
	m.Schedule = types.StringValue(w.Schedule)
	m.Timezone = types.StringValue(w.Timezone)
	m.Command = nil
	for _, c := range w.Command {
		m.Command = append(m.Command, types.StringValue(c))
	}
	m.HTTP = nil
	if w.HTTP != nil {
		h := &cronHTTPModel{Path: types.StringValue(w.HTTP.Path), Method: types.StringValue(w.HTTP.Method), Body: optString(w.HTTP.Body), Background: types.BoolValue(w.HTTP.Background)}
		for _, hd := range w.HTTP.Headers {
			h.Headers = append(h.Headers, cronHTTPHeaderModel{Name: types.StringValue(hd.Name), Value: optString(hd.Value), SecretRef: optString(hd.SecretRef), Prefix: optString(hd.Prefix)})
		}
		m.HTTP = h
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

// ValidateConfig requires exactly one of command or http, and one of value or secret_ref per header.
func (r *cronJobResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var command types.List
	var httpObj types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("command"), &command)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("http"), &httpObj)...)
	if resp.Diagnostics.HasError() || command.IsUnknown() || httpObj.IsUnknown() {
		return
	}
	if command.IsNull() == httpObj.IsNull() {
		resp.Diagnostics.AddError("Invalid cron job", "set exactly one of command or http")
		return
	}
	var headers types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("http").AtName("headers"), &headers)...)
	if headers.IsNull() || headers.IsUnknown() {
		return
	}
	for i, el := range headers.Elements() {
		obj, ok := el.(types.Object)
		if !ok || obj.IsUnknown() {
			continue
		}
		attrs := obj.Attributes()
		value, secret := attrs["value"], attrs["secret_ref"]
		if value.IsUnknown() || secret.IsUnknown() {
			continue
		}
		if value.IsNull() == secret.IsNull() {
			resp.Diagnostics.AddError("Invalid http header", fmt.Sprintf("headers[%d]: set exactly one of value or secret_ref", i))
		}
	}
}

// ModifyPlan keeps the computed mode in step with the http block, so a switch between command and http plans cleanly.
func (r *cronJobResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var httpObj types.Object
	var mode types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("http"), &httpObj)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("mode"), &mode)...)
	if resp.Diagnostics.HasError() {
		return
	}
	switch {
	case !httpObj.IsNull():
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("mode"), "http")...)
	case mode.ValueString() == "http":
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("mode"), types.StringUnknown())...)
	}
}

var (
	_ resource.ResourceWithValidateConfig = (*cronJobResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*cronJobResource)(nil)
)
