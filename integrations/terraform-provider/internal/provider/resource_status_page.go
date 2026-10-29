package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const statusPageID = "status-page"

// wireStatusPage mirrors the status page settings body of /api/v1/status-page.
type wireStatusPage struct {
	Enabled      bool   `json:"enabled"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CustomDomain string `json:"custom_domain"`
}

type statusPageResource struct{ resourceBase }

type statusPageModel struct {
	ID           types.String `tfsdk:"id"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	Title        types.String `tfsdk:"title"`
	Description  types.String `tfsdk:"description"`
	CustomDomain types.String `tfsdk:"custom_domain"`
}

// NewStatusPageResource returns the singleton status page settings resource.
func NewStatusPageResource() resource.Resource { return &statusPageResource{} }

func (r *statusPageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (r *statusPageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The public status page settings. There is one page per control plane.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Serve the public page."},
			"title":   schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Page title."},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Short text under the title.",
			},
			"custom_domain": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(""),
				Description: "Plain hostname that serves the page, such as status.example.com.",
			},
		},
	}
}

func (r *statusPageResource) put(ctx context.Context, m statusPageModel) error {
	body := wireStatusPage{
		Enabled: m.Enabled.ValueBool(), Title: m.Title.ValueString(),
		Description: m.Description.ValueString(), CustomDomain: m.CustomDomain.ValueString(),
	}
	return r.client.do(ctx, http.MethodPut, "/api/v1/status-page", body, nil)
}

func (r *statusPageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Save status page failed", err.Error())
		return
	}
	plan.ID = types.StringValue(statusPageID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *statusPageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out wireStatusPage
	if err := r.client.do(ctx, http.MethodGet, "/api/v1/status-page", nil, &out); err != nil {
		resp.Diagnostics.AddError("Read status page failed", err.Error())
		return
	}
	state.ID = types.StringValue(statusPageID)
	state.Enabled = types.BoolValue(out.Enabled)
	state.Title = types.StringValue(out.Title)
	state.Description = types.StringValue(out.Description)
	state.CustomDomain = types.StringValue(out.CustomDomain)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *statusPageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Save status page failed", err.Error())
		return
	}
	plan.ID = types.StringValue(statusPageID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete switches the page off and clears it, since the singleton cannot be removed.
func (r *statusPageResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.do(ctx, http.MethodPut, "/api/v1/status-page", wireStatusPage{}, nil); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Reset status page failed", err.Error())
	}
}

// ImportState accepts any ID because there is only one page.
func (r *statusPageResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), statusPageID)...)
}
