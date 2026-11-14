package provider

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type nodeLabelsResource struct{ resourceBase }

type nodeTaintModel struct {
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
	Effect types.String `tfsdk:"effect"`
}

type nodeLabelsModel struct {
	ID     types.String     `tfsdk:"id"`
	Node   types.String     `tfsdk:"node"`
	Labels types.Map        `tfsdk:"labels"`
	Taints []nodeTaintModel `tfsdk:"taints"`
}

type wireTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

// NewNodeLabelsResource returns the node labels and taints resource.
func NewNodeLabelsResource() resource.Resource { return &nodeLabelsResource{} }

func (r *nodeLabelsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_labels"
}

func (r *nodeLabelsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Labels and taints on an existing node. The node is never created or deleted here, and labels or taints set by other means are left alone. Destroying removes only what this resource set. Needs a root token.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Description: "The node ID.", Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node": schema.StringAttribute{
				Description:   "Name (or ID) of an existing node. Changing it replaces the resource.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"labels": schema.MapAttribute{
				Description: "Labels this resource owns on the node.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"taints": schema.ListNestedAttribute{
				Description: "Taints this resource owns on the node, identified by key and effect.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key":    schema.StringAttribute{Description: "Taint key.", Required: true},
					"value":  schema.StringAttribute{Description: "Taint value, empty when unset.", Optional: true},
					"effect": schema.StringAttribute{Description: "NoSchedule, PreferNoSchedule or NoExecute.", Required: true},
				}},
			},
		},
	}
}

// resolve returns the node ID for a name or ID, or a not-found APIError.
func (r *nodeLabelsResource) resolve(ctx context.Context, nameOrID string) (string, error) {
	var nodes []wireNode
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/nodes", nil, &nodes); err != nil {
		return "", err
	}
	for _, n := range nodes {
		if n.Name == nameOrID || n.ID == nameOrID {
			return n.ID, nil
		}
	}
	return "", &APIError{Status: http.StatusNotFound, Message: fmt.Sprintf("node %q not found", nameOrID)}
}

func (r *nodeLabelsResource) live(ctx context.Context, id string) (map[string]string, []wireTaint, error) {
	var lb struct {
		Labels map[string]string `json:"labels"`
	}
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/nodes/"+esc(id)+"/labels", nil, &lb); err != nil {
		return nil, nil, err
	}
	var tb struct {
		Taints []wireTaint `json:"taints"`
	}
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/nodes/"+esc(id)+"/taints", nil, &tb); err != nil {
		return nil, nil, err
	}
	if lb.Labels == nil {
		lb.Labels = map[string]string{}
	}
	return lb.Labels, tb.Taints, nil
}

func (m nodeLabelsModel) labelMap(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if m.Labels.IsNull() || m.Labels.IsUnknown() {
		return out, nil
	}
	if d := m.Labels.ElementsAs(ctx, &out, false); d.HasError() {
		return nil, fmt.Errorf("read labels: %v", d)
	}
	return out, nil
}

func taintID(key, effect string) string { return key + "\x00" + effect }

// apply sets want and drops what prev owned but want no longer has, leaving all other labels and taints as they are.
func (r *nodeLabelsResource) apply(ctx context.Context, id string, prev, want nodeLabelsModel) error {
	labels, taints, err := r.live(ctx, id)
	if err != nil {
		return err
	}
	prevL, err := prev.labelMap(ctx)
	if err != nil {
		return err
	}
	wantL, err := want.labelMap(ctx)
	if err != nil {
		return err
	}
	for k := range prevL {
		delete(labels, k)
	}
	for k, v := range wantL {
		labels[k] = v
	}
	drop := map[string]bool{}
	for _, t := range prev.Taints {
		drop[taintID(t.Key.ValueString(), t.Effect.ValueString())] = true
	}
	kept := make([]wireTaint, 0, len(taints))
	for _, t := range taints {
		if !drop[taintID(t.Key, t.Effect)] {
			kept = append(kept, t)
		}
	}
	for _, t := range want.Taints {
		id := taintID(t.Key.ValueString(), t.Effect.ValueString())
		kept = slices.DeleteFunc(kept, func(k wireTaint) bool { return taintID(k.Key, k.Effect) == id })
		kept = append(kept, wireTaint{Key: t.Key.ValueString(), Value: t.Value.ValueString(), Effect: t.Effect.ValueString()})
	}
	base := "/api/v1/nodes/" + esc(id)
	if err := r.client.do(ctx, http.MethodPut, base+"/labels", map[string]any{"labels": labels}, nil); err != nil {
		return err
	}
	return r.client.do(ctx, http.MethodPut, base+"/taints", map[string]any{"taints": kept}, nil)
}

func (r *nodeLabelsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeLabelsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.resolve(ctx, plan.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Find node failed", err.Error())
		return
	}
	if err := r.apply(ctx, id, nodeLabelsModel{}, plan); err != nil {
		resp.Diagnostics.AddError("Set node labels failed", err.Error())
		return
	}
	plan.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *nodeLabelsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeLabelsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	labels, taints, err := r.live(ctx, state.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read node labels failed", err.Error())
		return
	}
	// Only keys this resource owns are compared, so other writers never show as drift.
	owned, err := state.labelMap(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Read node labels failed", err.Error())
		return
	}
	if !state.Labels.IsNull() {
		seen := map[string]attr.Value{}
		for k := range owned {
			if v, ok := labels[k]; ok {
				seen[k] = types.StringValue(v)
			}
		}
		state.Labels = types.MapValueMust(types.StringType, seen)
	}
	if state.Taints != nil {
		var out []nodeTaintModel
		for _, want := range state.Taints {
			for _, t := range taints {
				if t.Key == want.Key.ValueString() && t.Effect == want.Effect.ValueString() {
					v := types.StringNull()
					if t.Value != "" || !want.Value.IsNull() {
						v = types.StringValue(t.Value)
					}
					out = append(out, nodeTaintModel{Key: want.Key, Value: v, Effect: want.Effect})
				}
			}
		}
		state.Taints = out
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *nodeLabelsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nodeLabelsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, state.ID.ValueString(), state, plan); err != nil {
		resp.Diagnostics.AddError("Set node labels failed", err.Error())
		return
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *nodeLabelsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeLabelsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.apply(ctx, state.ID.ValueString(), state, nodeLabelsModel{})
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Remove node labels failed", err.Error())
	}
}

// ImportState takes the node name or ID and adopts every live label and taint as owned.
func (r *nodeLabelsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := r.resolve(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Find node failed", err.Error())
		return
	}
	labels, taints, err := r.live(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Read node labels failed", err.Error())
		return
	}
	lv := map[string]attr.Value{}
	for k, v := range labels {
		lv[k] = types.StringValue(v)
	}
	var tm []nodeTaintModel
	for _, t := range taints {
		v := types.StringNull()
		if t.Value != "" {
			v = types.StringValue(t.Value)
		}
		tm = append(tm, nodeTaintModel{Key: types.StringValue(t.Key), Value: v, Effect: types.StringValue(t.Effect)})
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("labels"), types.MapValueMust(types.StringType, lv))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("taints"), tm)...)
}
