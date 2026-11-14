package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func providerImpl() *controlPlaneProvider { return New("test")().(*controlPlaneProvider) }

// TestSchemasDescribeEveryAttribute keeps the generated docs complete.
func TestSchemasDescribeEveryAttribute(t *testing.T) {
	ctx := context.Background()
	p := providerImpl()
	for _, mk := range p.Resources(ctx) {
		r := mk()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: TypeName}, &meta)
		var sr resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &sr)
		if sr.Schema.Description == "" {
			t.Errorf("%s: no description", meta.TypeName)
		}
		for name, a := range sr.Schema.Attributes {
			if a.GetDescription() == "" {
				t.Errorf("%s.%s: no description", meta.TypeName, name)
			}
			if n, ok := a.(schema.ListNestedAttribute); ok {
				for sub, sa := range n.NestedObject.Attributes {
					if sa.GetDescription() == "" {
						t.Errorf("%s.%s.%s: no description", meta.TypeName, name, sub)
					}
				}
			}
		}
	}
	for _, mk := range p.DataSources(ctx) {
		d := mk()
		var meta datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: TypeName}, &meta)
		var sr datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &sr)
		for name, a := range sr.Schema.Attributes {
			if a.GetDescription() == "" {
				t.Errorf("%s.%s: no description", meta.TypeName, name)
			}
		}
		_ = dsschema.Schema{}
	}
}

// TestEveryResourceImports checks each resource answers ImportState.
func TestEveryResourceImports(t *testing.T) {
	ctx := context.Background()
	for _, mk := range providerImpl().Resources(ctx) {
		if _, ok := mk().(resource.ResourceWithImportState); !ok {
			t.Errorf("%T has no ImportState", mk())
		}
	}
}

type fakeCP struct {
	app map[string]any
	db  map[string]any
	// calls records "METHOD path" in order.
	calls []string
}

func (f *fakeCP) serve(t *testing.T) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/api/v1/apps/web" && r.Method == http.MethodGet && f.app != nil:
			_ = json.NewEncoder(w).Encode(f.app)
		case r.URL.Path == "/api/v1/databases/main" && r.Method == http.MethodGet && f.db != nil:
			_ = json.NewEncoder(w).Encode(f.db)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/pooler"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "tok")
}

func nullState(ctx context.Context, r resource.Resource) tfsdk.State {
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	return tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}
}

func TestAppImportRoundTripAndDrift(t *testing.T) {
	ctx := context.Background()
	f := &fakeCP{app: map[string]any{
		"name": "web", "image": "nginx:1.27", "port": 80, "replicas": 2, "strategy": "rolling",
		"bind_address": "private", "project_id": "p1",
		"resources": map[string]any{"memory_bytes": 268435456, "nano_cpus": 500000000},
		"health":    map[string]any{"restart": "on-failure:3", "readiness": map[string]any{"path": "/ready"}},
	}}
	r := &appResource{}
	r.client = f.serve(t)

	imp := resource.ImportStateResponse{State: nullState(ctx, r)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "web"}, &imp)
	if imp.Diagnostics.HasError() {
		t.Fatal(imp.Diagnostics)
	}
	rd := resource.ReadResponse{State: imp.State}
	r.Read(ctx, resource.ReadRequest{State: imp.State}, &rd)
	if rd.Diagnostics.HasError() {
		t.Fatal(rd.Diagnostics)
	}
	var got appModel
	rd.State.Get(ctx, &got)
	if got.Image.ValueString() != "nginx:1.27" || got.MemoryBytes.ValueInt64() != 268435456 ||
		got.NanoCPUs.ValueInt64() != 500000000 || got.Restart.ValueString() != "on-failure:3" ||
		got.ReadinessPath.ValueString() != "/ready" || got.LivenessPath.ValueString() != "" {
		t.Fatalf("import did not read the live app: %+v", got)
	}
	if got.Command != nil || got.Labels != nil || got.PublishedPorts != nil {
		t.Fatal("unmanaged collections must stay null")
	}

	f.app = nil
	rd2 := resource.ReadResponse{State: rd.State}
	r.Read(ctx, resource.ReadRequest{State: rd.State}, &rd2)
	if !rd2.State.Raw.IsNull() {
		t.Fatal("a 404 must remove the app from state")
	}
}

func TestAppBodyKeepsLiveSettings(t *testing.T) {
	live := map[string]any{
		"resources": map[string]any{"memory_bytes": 1, "shm_size_bytes": 64},
		"health":    map[string]any{"readiness": map[string]any{"path": "/old", "interval": 5}},
	}
	m := appModel{Image: strVal("img")}
	m.MemoryBytes = int64Val(1024)
	m.ReadinessPath = strVal("/new")
	m.PublishedPorts = []publishedPortModel{{ContainerPort: int64Val(80), HostPort: int64Val(8080), Protocol: strVal("tcp"), Bind: strVal("")}}
	m.Command = []types.String{strVal("run"), strVal("--x")}
	body := m.body(live, false)
	res := body["resources"].(map[string]any)
	if res["memory_bytes"].(int64) != 1024 || res["shm_size_bytes"].(int) != 64 {
		t.Fatalf("resources not merged: %v", res)
	}
	rd := body["health"].(map[string]any)["readiness"].(map[string]any)
	if rd["path"] != "/new" || rd["interval"].(int) != 5 {
		t.Fatalf("probe not merged: %v", rd)
	}
	ports := body["published_ports"].([]map[string]any)
	if len(ports) != 1 || ports[0]["host_port"].(int64) != 8080 {
		t.Fatalf("ports: %v", ports)
	}
	if _, ok := ports[0]["bind"]; ok {
		t.Fatal("empty bind must be omitted")
	}
	if strings.Join(body["command"].([]string), " ") != "run --x" {
		t.Fatalf("command: %v", body["command"])
	}
}

func TestDatabaseImportRoundTripAndPooler(t *testing.T) {
	ctx := context.Background()
	f := &fakeCP{db: map[string]any{
		"name": "main", "engine": "postgres", "version": "16", "project_id": "p1",
		"pooler_enabled": true, "pooler_pool_size": 20, "pooler_max_client_conn": 200,
	}}
	r := &databaseResource{}
	r.client = f.serve(t)
	imp := resource.ImportStateResponse{State: nullState(ctx, r)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "main"}, &imp)
	rd := resource.ReadResponse{State: imp.State}
	r.Read(ctx, resource.ReadRequest{State: imp.State}, &rd)
	if rd.Diagnostics.HasError() {
		t.Fatal(rd.Diagnostics)
	}
	var got databaseModel
	rd.State.Get(ctx, &got)
	if got.Engine.ValueString() != "postgres" || !got.Pooler.ValueBool() || got.PoolSize.ValueInt64() != 20 || got.MaxConn.ValueInt64() != 200 {
		t.Fatalf("unexpected state %+v", got)
	}

	// Disabling the pooler on update calls DELETE; on create it is a no-op.
	off := databaseModel{Name: strVal("main"), Pooler: boolVal(false)}
	f.calls = nil
	if err := r.setPooler(ctx, off, true); err != nil || len(f.calls) != 1 || f.calls[0] != "DELETE /api/v1/databases/main/pooler" {
		t.Fatalf("calls %v err %v", f.calls, err)
	}
	f.calls = nil
	if err := r.setPooler(ctx, off, false); err != nil || len(f.calls) != 0 {
		t.Fatalf("create with pooler off must not call the API: %v %v", f.calls, err)
	}

	f.db = nil
	rd2 := resource.ReadResponse{State: rd.State}
	r.Read(ctx, resource.ReadRequest{State: rd.State}, &rd2)
	if !rd2.State.Raw.IsNull() {
		t.Fatal("a 404 must remove the database from state")
	}
}

func strVal(s string) types.String { return types.StringValue(s) }
func int64Val(i int64) types.Int64 { return types.Int64Value(i) }
func boolVal(b bool) types.Bool    { return types.BoolValue(b) }
