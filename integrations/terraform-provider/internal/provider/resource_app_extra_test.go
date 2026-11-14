package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func nullModel() appModel {
	return appModel{
		Name: strVal("web"), Image: strVal("img"),
		Port: types.Int64Null(), Replicas: types.Int64Null(),
		Strategy: types.StringNull(), BindAddress: types.StringNull(), ProjectID: types.StringNull(),
		Env: types.MapNull(types.StringType), Tags: types.SetNull(types.StringType),
		MemoryBytes: types.Int64Null(), NanoCPUs: types.Int64Null(),
		MemoryMB: types.Float64Null(), CPUs: types.Float64Null(),
		Restart: types.StringNull(), ReadinessPath: types.StringNull(), LivenessPath: types.StringNull(),
		ReadinessTCP: types.BoolNull(), LivenessTCP: types.BoolNull(),
	}
}

func validate(t *testing.T, m appModel) resource.ValidateConfigResponse {
	t.Helper()
	ctx := context.Background()
	r := &appResource{}
	st := nullState(ctx, r)
	if d := st.Set(ctx, m); d.HasError() {
		t.Fatal(d)
	}
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: st.Schema, Raw: st.Raw}}, &resp)
	return resp
}

func TestUnitConversionsAreExact(t *testing.T) {
	if mbToBytes(256) != 268435456 || mbToBytes(0.5) != 524288 || cpusToNano(0.5) != 500000000 || cpusToNano(1.25) != 1250000000 {
		t.Fatal("conversion mismatch")
	}
	m := nullModel()
	m.MemoryMB, m.CPUs = types.Float64Value(256), types.Float64Value(0.5)
	body := m.body(nil, true)
	res := body["resources"].(map[string]any)
	if res["memory_bytes"].(int64) != 268435456 || res["nano_cpus"].(int64) != 500000000 {
		t.Fatalf("body %v", res)
	}
	// The API answers in raw units; the friendly value the user wrote must survive.
	var w wireApp
	w.Resources.MemoryBytes, w.Resources.NanoCPUs = 268435456, 500000000
	m.unitsFromWire(w)
	if m.MemoryMB.ValueFloat64() != 256 || m.CPUs.ValueFloat64() != 0.5 {
		t.Fatalf("friendly units drifted: %v %v", m.MemoryMB, m.CPUs)
	}
	w.Resources.MemoryBytes = 536870912
	m.unitsFromWire(w)
	if m.MemoryMB.ValueFloat64() != 512 {
		t.Fatal("a live change must show as drift")
	}
	// Deprecated attributes keep working.
	old := nullModel()
	old.MemoryBytes, old.NanoCPUs = int64Val(1024), int64Val(2)
	r := old.body(nil, true)["resources"].(map[string]any)
	if r["memory_bytes"].(int64) != 1024 || r["nano_cpus"].(int64) != 2 {
		t.Fatalf("deprecated units: %v", r)
	}
}

func TestUnitAndProbeConflicts(t *testing.T) {
	ok := nullModel()
	ok.MemoryMB = types.Float64Value(64)
	if r := validate(t, ok); r.Diagnostics.HasError() {
		t.Fatal(r.Diagnostics)
	}
	bad := nullModel()
	bad.MemoryMB, bad.MemoryBytes = types.Float64Value(64), int64Val(1)
	if !validate(t, bad).Diagnostics.HasError() {
		t.Fatal("memory_mb with memory_bytes must conflict")
	}
	bad = nullModel()
	bad.CPUs, bad.NanoCPUs = types.Float64Value(1), int64Val(1)
	if !validate(t, bad).Diagnostics.HasError() {
		t.Fatal("cpus with nano_cpus must conflict")
	}
	bad = nullModel()
	bad.ReadinessPath, bad.ReadinessTCP = strVal("/x"), boolVal(true)
	if !validate(t, bad).Diagnostics.HasError() {
		t.Fatal("two readiness kinds must conflict")
	}
}

func TestProbeKindsBody(t *testing.T) {
	live := map[string]any{"health": map[string]any{"readiness": map[string]any{"path": "/old", "interval": 5}}}
	m := nullModel()
	m.ReadinessExec = []types.String{strVal("pg_isready")}
	m.LivenessTCP = boolVal(true)
	h := m.body(live, false)["health"].(map[string]any)
	rd := h["readiness"].(map[string]any)
	if _, has := rd["path"]; has || rd["interval"].(int) != 5 || rd["exec"].([]string)[0] != "pg_isready" {
		t.Fatalf("exec probe: %v", rd)
	}
	if h["liveness"].(map[string]any)["tcp"] != true {
		t.Fatalf("tcp probe: %v", h)
	}
	var w wireApp
	w.Health.Readiness = &wireProbe{Exec: []string{"pg_isready"}}
	w.Health.Liveness = &wireProbe{TCP: true}
	m.probesFromWire(w)
	if len(m.ReadinessExec) != 1 || !m.LivenessTCP.ValueBool() || m.ReadinessTCP.ValueBool() != false || m.ReadinessPath.ValueString() != "" {
		t.Fatalf("probe read back: %+v", m)
	}
}

func TestVolumesAndTagsSync(t *testing.T) {
	var calls []string
	var volBody map[string][]map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web/volumes":
			_ = json.NewDecoder(r.Body).Decode(&volBody)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/tags":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "t1", "name": "old"}, {"id": "t2", "name": "keep"}})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	r := &appResource{}
	r.client = NewClient(srv.URL, "tok")

	m := nullModel()
	m.Volumes = []volumeModel{{Name: strVal("data"), ContainerPath: strVal("/data")}}
	m.Tags = types.SetValueMust(types.StringType, []attr.Value{strVal("keep"), strVal("new")})
	if err := r.syncExtras(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if volBody["volumes"][0]["name"] != "data" || volBody["volumes"][0]["container_path"] != "/data" {
		t.Fatalf("volumes body %v", volBody)
	}
	got := strings.Join(calls, ",")
	if !strings.Contains(got, "DELETE /api/v1/apps/web/tags/t1") || !strings.Contains(got, "POST /api/v1/apps/web/tags") || strings.Contains(got, "t2") {
		t.Fatalf("tag calls: %s", got)
	}

	// Unmanaged volumes and tags make no calls.
	calls = nil
	if err := r.syncExtras(context.Background(), nullModel()); err != nil || len(calls) != 0 {
		t.Fatalf("unset must be untouched: %v %v", calls, err)
	}

	var w wireApp
	w.Volumes = append(w.Volumes, struct {
		Name          string `json:"name"`
		ContainerPath string `json:"container_path"`
	}{"data", "/data"})
	w.Tags = []string{"keep"}
	m.Volumes = volumesFromWire(m.Volumes, w)
	m.Tags = tagsFromWire(m.Tags, w)
	if len(m.Volumes) != 1 || len(m.Tags.Elements()) != 1 {
		t.Fatal("read back mismatch")
	}
	if volumesFromWire(nil, w) != nil || !tagsFromWire(types.SetNull(types.StringType), w).IsNull() {
		t.Fatal("unmanaged must stay null")
	}
}

func TestImportReadsFriendlyAndRawUnits(t *testing.T) {
	ctx := context.Background()
	f := &fakeCP{app: map[string]any{
		"name": "web", "image": "nginx", "resources": map[string]any{"memory_bytes": 268435456, "nano_cpus": 500000000},
		"volumes": []map[string]string{{"name": "data", "container_path": "/data"}}, "tags": []string{"a"},
	}}
	r := &appResource{}
	r.client = f.serve(t)
	imp := resource.ImportStateResponse{State: nullState(ctx, r)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "web"}, &imp)
	rd := resource.ReadResponse{State: imp.State}
	r.Read(ctx, resource.ReadRequest{State: imp.State}, &rd)
	if rd.Diagnostics.HasError() {
		t.Fatal(rd.Diagnostics)
	}
	var got appModel
	rd.State.Get(ctx, &got)
	if got.MemoryBytes.ValueInt64() != 268435456 || !got.MemoryMB.IsNull() || !got.CPUs.IsNull() || got.Volumes != nil || !got.Tags.IsNull() {
		t.Fatalf("import state: %+v", got)
	}
}

func TestAppSchemaDescribesAndDeprecates(t *testing.T) {
	var sr resource.SchemaResponse
	(&appResource{}).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	for name, a := range sr.Schema.Attributes {
		if a.GetDescription() == "" {
			t.Errorf("attribute %s has no description", name)
		}
	}
	for _, n := range []string{"memory_bytes", "nano_cpus"} {
		if sr.Schema.Attributes[n].GetDeprecationMessage() == "" {
			t.Errorf("%s must be deprecated", n)
		}
	}
	for _, n := range []string{"memory_mb", "cpus", "volumes", "tags", "readiness_exec", "readiness_tcp", "liveness_exec", "liveness_tcp"} {
		if sr.Schema.Attributes[n] == nil {
			t.Errorf("missing %s", n)
		}
	}
}
