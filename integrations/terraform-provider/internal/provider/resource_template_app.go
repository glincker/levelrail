package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type templateAppResource struct{ resourceBase }

type templateAppModel struct {
	ID       types.String `tfsdk:"id"`
	Template types.String `tfsdk:"template"`
	Name     types.String `tfsdk:"name"`
	Env      types.Map    `tfsdk:"env"`
	Domains  types.Map    `tfsdk:"domains"`
}

// NewTemplateAppResource returns the catalog template install resource.
func NewTemplateAppResource() resource.Resource { return &templateAppResource{} }

func (r *templateAppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template_app"
}

func (r *templateAppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A catalog template installed by id, the same install the dashboard form and a TemplateApp document run. env and domains apply at install time only: changing one destroys the app and installs it again onto the surviving named volumes (regenerated passwords can break stateful apps), and a protected app refuses the delete. Use lifecycle prevent_destroy and ignore_changes for env and domains. Terraform state keeps env in plaintext, Sensitive only masks plan output.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Description: "Identifier assigned by the provider, the app name.", Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"template": schema.StringAttribute{Description: "Catalog template id.", Required: true, PlanModifiers: replace},
			"name": schema.StringAttribute{Description: "App name. Defaults to the template id.", Optional: true, Computed: true, PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}},
			"env": schema.MapAttribute{Description: "Values for the template install form. Keys must be settings the template declares.", Optional: true, Sensitive: true, ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplaceIf(replaceIfSet, "Replaced when a value that was installed changes.", "Replaced when a value that was installed changes.")}},
			"domains": schema.MapAttribute{Description: "Host per service the template offers a domain for.", Optional: true, ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplaceIf(replaceIfSet, "Replaced when a value that was installed changes.", "Replaced when a value that was installed changes.")}},
		},
	}
}

// replaceIfSet skips the replace when the prior state holds no value, as after an import, which cannot read install inputs back.
func replaceIfSet(_ context.Context, req planmodifier.MapRequest, resp *mapplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.StateValue.IsNull() && !req.StateValue.Equal(req.PlanValue)
}

func stringMap(ctx context.Context, m types.Map) (map[string]string, error) {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}
	if d := m.ElementsAs(ctx, &out, false); d.HasError() {
		return nil, fmt.Errorf("read map: %s", d.Errors()[0].Detail())
	}
	return out, nil
}

func (r *templateAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan templateAppModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := plan.Name.ValueString()
	if plan.Name.IsNull() || plan.Name.IsUnknown() || name == "" {
		name = plan.Template.ValueString()
	}
	env, err := stringMap(ctx, plan.Env)
	if err == nil {
		var domains map[string]string
		if domains, err = stringMap(ctx, plan.Domains); err == nil {
			body := map[string]any{"name": name}
			if len(env) > 0 {
				body["env"] = env
			}
			if len(domains) > 0 {
				body["domains"] = domains
			}
			err = r.client.do(ctx, http.MethodPost, "/api/v1/service-templates/"+esc(plan.Template.ValueString())+"/install", body, nil)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Install template failed", err.Error())
		return
	}
	plan.Name = types.StringValue(name)
	plan.ID = types.StringValue(name)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *templateAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state templateAppModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var list struct {
		Revisions []struct {
			Source     string `json:"source"`
			TemplateID string `json:"template_id"`
		} `json:"revisions"`
	}
	err := r.client.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(state.Name.ValueString())+"/compose/revisions", nil, &list)
	if isNotFound(err) || (err == nil && len(list.Revisions) == 0) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read template app failed", err.Error())
		return
	}
	// After an import the template is unknown, so read it from the latest revision.
	if state.Template.IsNull() && list.Revisions[0].Source == "template" {
		state.Template = types.StringValue(list.Revisions[0].TemplateID)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only runs for computed drift, every input forces a replace.
func (r *templateAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan templateAppModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *templateAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state templateAppModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.do(ctx, http.MethodDelete, "/api/v1/apps/"+esc(state.Name.ValueString())+"/compose", nil, nil)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Remove template app failed", err.Error())
	}
}

// ImportState takes the app name.
func (r *templateAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
