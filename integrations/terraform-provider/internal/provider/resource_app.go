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

	Command        []types.String          `tfsdk:"command"`
	Labels         map[string]types.String `tfsdk:"labels"`
	MemoryBytes    types.Int64             `tfsdk:"memory_bytes"`
	NanoCPUs       types.Int64             `tfsdk:"nano_cpus"`
	Restart        types.String            `tfsdk:"restart"`
	ReadinessPath  types.String            `tfsdk:"readiness_path"`
	LivenessPath   types.String            `tfsdk:"liveness_path"`
	PublishedPorts []publishedPortModel    `tfsdk:"published_ports"`

	MemoryMB      types.Float64  `tfsdk:"memory_mb"`
	CPUs          types.Float64  `tfsdk:"cpus"`
	ReadinessExec []types.String `tfsdk:"readiness_exec"`
	ReadinessTCP  types.Bool     `tfsdk:"readiness_tcp"`
	LivenessExec  []types.String `tfsdk:"liveness_exec"`
	LivenessTCP   types.Bool     `tfsdk:"liveness_tcp"`
	Volumes       []volumeModel  `tfsdk:"volumes"`
	Tags          types.Set      `tfsdk:"tags"`
}

type publishedPortModel struct {
	ContainerPort types.Int64  `tfsdk:"container_port"`
	HostPort      types.Int64  `tfsdk:"host_port"`
	Protocol      types.String `tfsdk:"protocol"`
	Bind          types.String `tfsdk:"bind"`
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
				Description:   "Name of the resource.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"image": schema.StringAttribute{Required: true, Description: "Container image reference, for example nginx:1.27."},
			"port": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Container port the app listens on.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"replicas": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Number of running instances.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"strategy":     schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, Description: "Rollout strategy, for example rolling or recreate."},
			"bind_address": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, Description: "Host interface the app port binds to."},
			"project_id":   schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, Description: "Project the app belongs to."},
			"command": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Overrides the image command. Unset leaves the live command untouched.",
			},
			"labels": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Container labels. Unset leaves live labels untouched.",
			},
			"memory_mb": schema.Float64Attribute{
				Optional:    true,
				Description: "Memory limit in MiB, for example 256. 0 means no limit. Preferred over memory_bytes; set only one of them.",
			},
			"cpus": schema.Float64Attribute{
				Optional:    true,
				Description: "CPU limit as a number of CPUs, fractions allowed, for example 0.5. 0 means no limit. Preferred over nano_cpus; set only one of them.",
			},
			"memory_bytes": schema.Int64Attribute{
				Optional: true, Computed: true,
				DeprecationMessage: "Use memory_mb instead.",
				PlanModifiers:      []planmodifier.Int64{unitFollows{sibling: "memory_mb"}},
				Description:        "Memory limit in bytes. 0 means no limit.",
			},
			"nano_cpus": schema.Int64Attribute{
				Optional: true, Computed: true,
				DeprecationMessage: "Use cpus instead.",
				PlanModifiers:      []planmodifier.Int64{unitFollows{sibling: "cpus"}},
				Description:        "CPU limit in billionths of a CPU (1000000000 is one CPU). 0 means no limit.",
			},
			"restart":        schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, Description: "Restart policy: no or on-failure[:N]. Empty keeps the app running."},
			"readiness_path": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: probeKeep("readiness"), Description: "HTTP path of the readiness probe. Empty means no probe. Not combined with readiness_exec or readiness_tcp."},
			"liveness_path":  schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: probeKeep("liveness"), Description: "HTTP path of the liveness probe. Empty means no probe. Not combined with liveness_exec or liveness_tcp."},
			"readiness_exec": schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Readiness probe as a command run inside the container, healthy on exit 0. Unset leaves the live probe untouched."},
			"readiness_tcp":  schema.BoolAttribute{Optional: true, Description: "Readiness probe that only opens a TCP connection to the app port. Unset leaves the live probe untouched."},
			"liveness_exec":  schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Liveness probe as a command run inside the container, healthy on exit 0. Unset leaves the live probe untouched."},
			"liveness_tcp":   schema.BoolAttribute{Optional: true, Description: "Liveness probe that only opens a TCP connection to the app port. Unset leaves the live probe untouched."},
			"volumes": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Named volumes mounted into the container. Unset leaves live volumes untouched; an empty list detaches them all.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name":           schema.StringAttribute{Required: true, Description: "Volume name: lowercase letters, digits and hyphens, starting with a letter."},
					"container_path": schema.StringAttribute{Required: true, Description: "Absolute mount path inside the container."},
				}},
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Tags on the app, lowercase, as key or key:value. Unset leaves live tags untouched; an empty set removes them all.",
			},
			"published_ports": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Extra host ports published for the app. Unset leaves live ports untouched.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"container_port": schema.Int64Attribute{Required: true, Description: "Port inside the container."},
					"host_port":      schema.Int64Attribute{Required: true, Description: "Port on the host."},
					"protocol":       schema.StringAttribute{Optional: true, Computed: true, Description: "tcp or udp. Defaults to tcp."},
					"bind":           schema.StringAttribute{Optional: true, Computed: true, Description: "private or public for this port. Empty follows bind_address."},
				}},
			},
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
	m.collectionsBody(body)
	return body
}

// subMap returns the live nested object so partial changes keep its other keys.
func subMap(body map[string]any, key string) map[string]any {
	if m, ok := body[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	body[key] = m
	return m
}

func (m appModel) collectionsBody(body map[string]any) {
	if m.Command != nil {
		cmd := make([]string, 0, len(m.Command))
		for _, c := range m.Command {
			cmd = append(cmd, c.ValueString())
		}
		body["command"] = cmd
	}
	if m.Labels != nil {
		labels := map[string]string{}
		for k, v := range m.Labels {
			labels[k] = v.ValueString()
		}
		body["labels"] = labels
	}
	if m.PublishedPorts != nil {
		ports := make([]map[string]any, 0, len(m.PublishedPorts))
		for _, p := range m.PublishedPorts {
			e := map[string]any{"container_port": p.ContainerPort.ValueInt64(), "host_port": p.HostPort.ValueInt64()}
			if s := p.Protocol.ValueString(); s != "" {
				e["protocol"] = s
			}
			if s := p.Bind.ValueString(); s != "" {
				e["bind"] = s
			}
			ports = append(ports, e)
		}
		body["published_ports"] = ports
	}
	known := func(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }
	if known(m.MemoryMB) {
		subMap(body, "resources")["memory_bytes"] = mbToBytes(m.MemoryMB.ValueFloat64())
	} else if known(m.MemoryBytes) {
		subMap(body, "resources")["memory_bytes"] = m.MemoryBytes.ValueInt64()
	}
	if known(m.CPUs) {
		subMap(body, "resources")["nano_cpus"] = cpusToNano(m.CPUs.ValueFloat64())
	} else if known(m.NanoCPUs) {
		subMap(body, "resources")["nano_cpus"] = m.NanoCPUs.ValueInt64()
	}
	m.probesBody(body)
	if known(m.Restart) {
		subMap(body, "health")["restart"] = m.Restart.ValueString()
	}
}

func (m *appModel) fromWire(w wireApp) {
	m.Image = types.StringValue(w.Image)
	m.Port = types.Int64Value(w.Port)
	m.Replicas = types.Int64Value(w.Replicas)
	m.Strategy = types.StringValue(w.Strategy)
	m.BindAddress = types.StringValue(w.BindAddress)
	m.ProjectID = types.StringValue(w.ProjectID)
	m.collectionsFromWire(w)
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
	if err := r.syncExtras(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Update app volumes or tags failed", err.Error())
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
	if err := r.syncExtras(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Update app volumes or tags failed", err.Error())
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

// collectionsFromWire refreshes lists and maps only when configured, so an
// unset attribute never shows live values the user chose not to manage.
func (m *appModel) collectionsFromWire(w wireApp) {
	m.MemoryBytes = types.Int64Value(w.Resources.MemoryBytes)
	m.NanoCPUs = types.Int64Value(w.Resources.NanoCPUs)
	m.unitsFromWire(w)
	m.probesFromWire(w)
	m.Volumes = volumesFromWire(m.Volumes, w)
	m.Tags = tagsFromWire(m.Tags, w)
	m.Restart = types.StringValue(w.Health.Restart)
	if m.Command != nil {
		m.Command = make([]types.String, 0, len(w.Command))
		for _, c := range w.Command {
			m.Command = append(m.Command, types.StringValue(c))
		}
	}
	if m.Labels != nil {
		m.Labels = make(map[string]types.String, len(w.Labels))
		for k, v := range w.Labels {
			m.Labels[k] = types.StringValue(v)
		}
	}
	if m.PublishedPorts != nil {
		m.PublishedPorts = make([]publishedPortModel, 0, len(w.PublishedPorts))
		for _, p := range w.PublishedPorts {
			m.PublishedPorts = append(m.PublishedPorts, publishedPortModel{
				ContainerPort: types.Int64Value(p.ContainerPort),
				HostPort:      types.Int64Value(p.HostPort),
				Protocol:      types.StringValue(p.Protocol),
				Bind:          types.StringValue(p.Bind),
			})
		}
	}
}
