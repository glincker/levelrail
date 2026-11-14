package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type settingResource struct{ resourceBase }

type settingModel struct {
	ID    types.String `tfsdk:"id"`
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

// wireSetting is the part of a runtime setting the provider needs.
type wireSetting struct {
	Key       string          `json:"key"`
	Kind      string          `json:"kind"`
	Value     json.RawMessage `json:"value"`
	Source    string          `json:"source"`
	Sensitive bool            `json:"sensitive"`
}

// NewSettingResource returns the runtime setting resource.
func NewSettingResource() resource.Resource { return &settingResource{} }

func (r *settingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting"
}

func (r *settingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "One runtime setting. Destroying it resets the setting to its default. Settings marked sensitive are refused: manage them from the CLI so a credential never lands in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Description: "The setting key.", Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"key": schema.StringAttribute{
				Description:   "Setting key, for example `deploy.inline_source_keep`. Only keys the control plane lists are accepted. Changing it replaces the resource.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				Description: "Value as text: `true`, `42`, `15m`, plain text, or a JSON array for a list setting.",
				Required:    true,
			},
		},
	}
}

func (r *settingResource) get(ctx context.Context, key string) (wireSetting, error) {
	var s wireSetting
	err := r.client.do(ctx, http.MethodGet, "/api/v1/settings/runtime/"+esc(key), nil, &s)
	return s, err
}

// settingJSON turns the text value into the JSON the API expects for the kind.
func settingJSON(kind, v string) (json.RawMessage, error) {
	switch kind {
	case "bool":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("value %q is not a boolean", v)
		}
		return json.Marshal(b)
	case "int":
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("value %q is not an integer", v)
		}
		return json.Marshal(n)
	case "string_list":
		if !json.Valid([]byte(v)) {
			return nil, fmt.Errorf("value %q is not a JSON array", v)
		}
		return json.RawMessage(v), nil
	}
	return json.Marshal(v)
}

// settingText is the inverse of settingJSON.
func settingText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

func (r *settingResource) write(ctx context.Context, m settingModel) error {
	key := m.Key.ValueString()
	live, err := r.get(ctx, key)
	if err != nil {
		return fmt.Errorf("look up setting: %w", err)
	}
	if live.Sensitive {
		return fmt.Errorf("setting %q is sensitive and cannot be managed from Terraform state", key)
	}
	val, err := settingJSON(live.Kind, m.Value.ValueString())
	if err != nil {
		return err
	}
	return r.client.do(ctx, http.MethodPut, "/api/v1/settings/runtime/"+esc(key), map[string]any{"value": val}, nil)
}

func (r *settingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan settingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Set setting failed", err.Error())
		return
	}
	plan.ID = plan.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *settingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state settingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	live, err := r.get(ctx, state.Key.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read setting failed", err.Error())
		return
	}
	if live.Sensitive {
		resp.Diagnostics.AddError("Sensitive setting", fmt.Sprintf("setting %q is sensitive and cannot be managed from Terraform state", live.Key))
		return
	}
	state.Value = types.StringValue(settingText(live.Value))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *settingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan settingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Set setting failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *settingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state settingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/settings/runtime/"+esc(state.Key.ValueString()), nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Reset setting failed", err.Error())
	}
}

// ImportState takes the setting key.
func (r *settingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}
