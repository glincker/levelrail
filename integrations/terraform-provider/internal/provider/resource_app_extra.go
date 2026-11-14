package provider

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const bytesPerMiB = 1 << 20

type volumeModel struct {
	Name          types.String `tfsdk:"name"`
	ContainerPath types.String `tfsdk:"container_path"`
}

func mbToBytes(mb float64) int64    { return int64(math.Round(mb * bytesPerMiB)) }
func cpusToNano(cpus float64) int64 { return int64(math.Round(cpus * 1e9)) }
func bytesToMB(b int64) float64     { return float64(b) / bytesPerMiB }
func nanoToCPUs(nano int64) float64 { return float64(nano) / 1e9 }
func knownVal(v attr.Value) bool    { return !v.IsNull() && !v.IsUnknown() }

// unitsFromWire keeps a friendly value the user configured unless the live
// value no longer matches it, so float rounding never shows a diff.
func (m *appModel) unitsFromWire(w wireApp) {
	if knownVal(m.MemoryMB) && mbToBytes(m.MemoryMB.ValueFloat64()) != w.Resources.MemoryBytes {
		m.MemoryMB = types.Float64Value(bytesToMB(w.Resources.MemoryBytes))
	}
	if knownVal(m.CPUs) && cpusToNano(m.CPUs.ValueFloat64()) != w.Resources.NanoCPUs {
		m.CPUs = types.Float64Value(nanoToCPUs(w.Resources.NanoCPUs))
	}
}

// unitFollows marks the deprecated raw-unit attribute unknown only when its
// friendly sibling changes, so an unchanged config shows no diff.
type unitFollows struct{ sibling string }

func (u unitFollows) Description(context.Context) string {
	return "Follows " + u.sibling + " when that is set."
}
func (u unitFollows) MarkdownDescription(ctx context.Context) string { return u.Description(ctx) }

func (u unitFollows) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.StateValue.IsNull() {
		return
	}
	var cfg, prior types.Float64
	req.Config.GetAttribute(ctx, path.Root(u.sibling), &cfg)
	req.State.GetAttribute(ctx, path.Root(u.sibling), &prior)
	if !cfg.IsNull() && !cfg.Equal(prior) {
		return
	}
	resp.PlanValue = req.StateValue
}

// probeKeep keeps the stored path unless an exec or tcp probe replaces it.
func probeKeep(kind string) []planmodifier.String {
	return []planmodifier.String{probeKeepMod{kind: kind}, stringplanmodifier.UseStateForUnknown()}
}

type probeKeepMod struct{ kind string }

func (p probeKeepMod) Description(context.Context) string {
	return "Becomes unknown when an exec or tcp probe is configured."
}
func (p probeKeepMod) MarkdownDescription(ctx context.Context) string { return p.Description(ctx) }

func (p probeKeepMod) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	var exec types.List
	var tcp types.Bool
	req.Config.GetAttribute(ctx, path.Root(p.kind+"_exec"), &exec)
	req.Config.GetAttribute(ctx, path.Root(p.kind+"_tcp"), &tcp)
	if (!exec.IsNull() && len(exec.Elements()) > 0) || (!tcp.IsNull() && tcp.ValueBool()) {
		resp.PlanValue = types.StringValue("")
	}
}

// ValidateConfig rejects a friendly and a raw unit together, and mixed probe kinds.
func (r *appResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg appModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	set := func(v attr.Value) bool { return !v.IsNull() }
	pairs := [][2]struct {
		name string
		v    attr.Value
	}{
		{{"memory_mb", cfg.MemoryMB}, {"memory_bytes", cfg.MemoryBytes}},
		{{"cpus", cfg.CPUs}, {"nano_cpus", cfg.NanoCPUs}},
	}
	for _, p := range pairs {
		if set(p[0].v) && set(p[1].v) {
			resp.Diagnostics.AddAttributeError(path.Root(p[0].name), "Conflicting attributes",
				fmt.Sprintf("%s and %s cannot both be set; use %s.", p[0].name, p[1].name, p[0].name))
		}
	}
	for _, k := range []struct {
		kind string
		path types.String
		exec []types.String
		tcp  types.Bool
	}{
		{"readiness", cfg.ReadinessPath, cfg.ReadinessExec, cfg.ReadinessTCP},
		{"liveness", cfg.LivenessPath, cfg.LivenessExec, cfg.LivenessTCP},
	} {
		kinds := 0
		if knownVal(k.path) && k.path.ValueString() != "" {
			kinds++
		}
		if len(k.exec) > 0 {
			kinds++
		}
		if knownVal(k.tcp) && k.tcp.ValueBool() {
			kinds++
		}
		if kinds > 1 {
			resp.Diagnostics.AddAttributeError(path.Root(k.kind+"_path"), "Conflicting attributes",
				fmt.Sprintf("Set only one of %[1]s_path, %[1]s_exec and %[1]s_tcp.", k.kind))
		}
	}
}

func (m appModel) probesBody(body map[string]any) {
	for _, k := range []struct {
		key  string
		path types.String
		exec []types.String
		tcp  types.Bool
	}{
		{"readiness", m.ReadinessPath, m.ReadinessExec, m.ReadinessTCP},
		{"liveness", m.LivenessPath, m.LivenessExec, m.LivenessTCP},
	} {
		switch {
		case len(k.exec) > 0:
			probe := subMap(subMap(body, "health"), k.key)
			cmd := make([]string, 0, len(k.exec))
			for _, c := range k.exec {
				cmd = append(cmd, c.ValueString())
			}
			delete(probe, "path")
			delete(probe, "tcp")
			probe["exec"] = cmd
		case knownVal(k.tcp) && k.tcp.ValueBool():
			probe := subMap(subMap(body, "health"), k.key)
			delete(probe, "path")
			delete(probe, "exec")
			probe["tcp"] = true
		case knownVal(k.path) && k.path.ValueString() != "":
			probe := subMap(subMap(body, "health"), k.key)
			delete(probe, "exec")
			delete(probe, "tcp")
			probe["path"] = k.path.ValueString()
		}
	}
}

func (m *appModel) probesFromWire(w wireApp) {
	m.ReadinessPath, m.LivenessPath = types.StringValue(""), types.StringValue("")
	if p := w.Health.Readiness; p != nil {
		m.ReadinessPath = types.StringValue(p.Path)
	}
	if p := w.Health.Liveness; p != nil {
		m.LivenessPath = types.StringValue(p.Path)
	}
	m.ReadinessExec, m.ReadinessTCP = execFromWire(m.ReadinessExec, m.ReadinessTCP, w.Health.Readiness)
	m.LivenessExec, m.LivenessTCP = execFromWire(m.LivenessExec, m.LivenessTCP, w.Health.Liveness)
}

// execFromWire refreshes exec and tcp only when configured, like other optional collections.
func execFromWire(exec []types.String, tcp types.Bool, p *wireProbe) ([]types.String, types.Bool) {
	if exec != nil {
		exec = []types.String{}
		if p != nil {
			for _, c := range p.Exec {
				exec = append(exec, types.StringValue(c))
			}
		}
	}
	if !tcp.IsNull() {
		tcp = types.BoolValue(p != nil && p.TCP)
	}
	return exec, tcp
}

func volumesFromWire(cur []volumeModel, w wireApp) []volumeModel {
	if cur == nil {
		return nil
	}
	out := make([]volumeModel, 0, len(w.Volumes))
	for _, v := range w.Volumes {
		out = append(out, volumeModel{Name: types.StringValue(v.Name), ContainerPath: types.StringValue(v.ContainerPath)})
	}
	return out
}

func tagsFromWire(cur types.Set, w wireApp) types.Set {
	if cur.IsNull() || cur.IsUnknown() {
		return cur
	}
	elems := make([]attr.Value, 0, len(w.Tags))
	for _, t := range w.Tags {
		elems = append(elems, types.StringValue(t))
	}
	return types.SetValueMust(types.StringType, elems)
}

// syncExtras writes volumes and tags, which have their own routes and are
// not part of the app body.
func (r *appResource) syncExtras(ctx context.Context, plan appModel) error {
	name := esc(plan.Name.ValueString())
	if plan.Volumes != nil {
		vols := make([]map[string]string, 0, len(plan.Volumes))
		for _, v := range plan.Volumes {
			vols = append(vols, map[string]string{"name": v.Name.ValueString(), "container_path": v.ContainerPath.ValueString()})
		}
		if err := r.client.do(ctx, http.MethodPut, "/api/v1/apps/"+name+"/volumes", map[string]any{"volumes": vols}, nil); err != nil {
			return err
		}
	}
	if plan.Tags.IsNull() || plan.Tags.IsUnknown() {
		return nil
	}
	var live []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/apps/"+name+"/tags", nil, &live); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, e := range plan.Tags.Elements() {
		if s, ok := e.(types.String); ok {
			want[s.ValueString()] = true
		}
	}
	have := map[string]bool{}
	for _, t := range live {
		have[t.Name] = true
		if !want[t.Name] {
			if err := r.client.do(ctx, http.MethodDelete, "/api/v1/apps/"+name+"/tags/"+esc(t.ID), nil, nil); err != nil {
				return err
			}
		}
	}
	add := make([]string, 0, len(want))
	for t := range want {
		if !have[t] {
			add = append(add, t)
		}
	}
	sort.Strings(add)
	for _, t := range add {
		if err := r.client.do(ctx, http.MethodPost, "/api/v1/apps/"+name+"/tags", map[string]string{"name": t}, nil); err != nil {
			return err
		}
	}
	return nil
}

var _ resource.ResourceWithValidateConfig = (*appResource)(nil)
