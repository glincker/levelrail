package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestProjectsDataSourceListsProjects(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"p1","name":"shop"},{"id":"p2","name":"blog"}]`))
	}))
	defer srv.Close()
	d := &projectsDataSource{}
	d.client = NewClient(srv.URL, "tok")

	var sr datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sr)
	raw := tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: sr.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}
	var got projectsModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if len(got.Projects) != 2 || got.Projects[0].Name.ValueString() != "shop" {
		t.Fatalf("unexpected listing: %+v", got.Projects)
	}
}
