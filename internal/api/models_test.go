package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newModelsTestRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	svc := models.NewService(db, nil, models.NewHostResolver("", nil), "")
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithModels(svc)), db
}

func doModels(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestModelsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newModelsTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/models"},
		{http.MethodPost, "/api/v1/models"},
		{http.MethodGet, "/api/v1/models/chat"},
		{http.MethodDelete, "/api/v1/models/chat"},
		{http.MethodPost, "/api/v1/models/chat/restart"},
		{http.MethodPost, "/api/v1/models/chat/api-key"},
		{http.MethodPut, "/api/v1/models/chat/hf-token"},
		{http.MethodGet, "/api/v1/models/chat/logs"},
		{http.MethodGet, "/api/v1/models/chat/engine-metrics"},
		{http.MethodGet, "/api/v1/models/chat/fit"},
		{http.MethodPut, "/api/v1/models/chat/residency"},
		{http.MethodPut, "/api/v1/models/chat/swap-group"},
		{http.MethodPost, "/api/v1/models/chat/wake"},
		{http.MethodPost, "/api/v1/models/chat/sleep"},
		{http.MethodPost, "/api/v1/models/fit"},
		{http.MethodGet, "/api/v1/models/chat/logs/stream"},
		{http.MethodGet, "/api/v1/gpus"},
	})
}

func TestModelsRoutes_AbilityGuards(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "t1", Name: "ro", TokenHash: hashToken("ro-secret"), Abilities: []string{AbilityRead}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "t2", Name: "rw", TokenHash: hashToken("rw-secret"), Abilities: []string{AbilityWrite}}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, target, body, token string
		want                              int
	}{
		{"read token can list", http.MethodGet, "/api/v1/models", "", "ro-secret", http.StatusOK},
		{"read token can list gpus", http.MethodGet, "/api/v1/gpus", "", "ro-secret", http.StatusOK},
		{"read token cannot create", http.MethodPost, "/api/v1/models", `{}`, "ro-secret", http.StatusForbidden},
		{"write token cannot create (needs write:sensitive)", http.MethodPost, "/api/v1/models", `{}`, "rw-secret", http.StatusForbidden},
		{"write token cannot rotate key", http.MethodPost, "/api/v1/models/x/api-key", "", "rw-secret", http.StatusForbidden},
		{"write token cannot set hf token", http.MethodPut, "/api/v1/models/x/hf-token", `{}`, "rw-secret", http.StatusForbidden},
		{"read token cannot set swap group", http.MethodPut, "/api/v1/models/x/swap-group", `{}`, "ro-secret", http.StatusForbidden},
		{"write token reaches swap group (404, not 403)", http.MethodPut, "/api/v1/models/x/swap-group", `{"swap_group":"gpu0"}`, "rw-secret", http.StatusNotFound},
		{"read token cannot delete", http.MethodDelete, "/api/v1/models/x", "", "ro-secret", http.StatusForbidden},
		{"write token reaches delete (404, not 403)", http.MethodDelete, "/api/v1/models/x", "", "rw-secret", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, bearerRequest(tt.method, tt.target, tt.body, tt.token))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestModelsRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, target := range []string{"/api/v1/models", "/api/v1/models/x", "/api/v1/gpus"} {
		if rec := doModels(t, rt, cookie, http.MethodGet, target, ""); rec.Code != http.StatusNotImplemented {
			t.Errorf("GET %s status = %d, want 501", target, rec.Code)
		}
	}
}

func TestModels_CreateGetListDeleteFlow(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"chat","engine":"ollama","model":"llama3.1:8b","domain":"chat.example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created createModelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.APIKey, "lr-") || created.Name != "chat" || created.EndpointURL != "https://chat.example.com/v1" || created.GPUCount != -1 {
		t.Errorf("created = %+v", created)
	}

	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat", "")
	if strings.Contains(rec.Body.String(), created.APIKey) || strings.Contains(rec.Body.String(), "api_key\"") {
		t.Errorf("GET must never return the API key: %s", rec.Body.String())
	}
	var got modelResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.APIKeyPrefix != created.APIKey[:8] || got.Status.Reason != "Pending" {
		t.Errorf("got = %+v", got)
	}
	if got.Limits != models.DefaultGatewayLimits().Summary() {
		t.Errorf("limits = %+v, want the gateway defaults", got.Limits)
	}

	if err := db.UpsertConditions(context.Background(), models.ControllerName("chat"), []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "Downloading", Message: "downloading llama3.1:8b: 12%"}}); err != nil {
		t.Fatal(err)
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models", "")
	var list []modelResource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Status.Reason != "Downloading" || list[0].Status.Ready || !strings.Contains(list[0].Status.Message, "12%") {
		t.Errorf("list = %+v", list)
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/api-key", "")
	var rotated modelAPIKeyResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &rotated)
	if rec.Code != http.StatusOK || rotated.APIKey == "" || rotated.APIKey == created.APIKey {
		t.Errorf("rotate = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/restart", ""); rec.Code != http.StatusNoContent {
		t.Errorf("restart status = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodDelete, "/api/v1/models/chat", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete status = %d", rec.Code)
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Status.Reason != "Deleting" {
		t.Errorf("after delete reason = %q, want Deleting", got.Status.Reason)
	}
}

func TestModels_CreateErrors(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	valid := `{"name":"chat","engine":"ollama","model":"llama3.1:8b"}`
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", valid); rec.Code != http.StatusCreated {
		t.Fatalf("seed create: %d", rec.Code)
	}
	tests := []struct {
		name, body string
		want       int
		contains   string
	}{
		{"bad json", `{`, 400, "invalid request body"},
		{"unknown engine", `{"name":"a","engine":"tgi","model":"x"}`, 400, "not supported"},
		{"bad model for engine", `{"name":"a","engine":"vllm","model":"llama3:8b"}`, 400, "HuggingFace"},
		{"duplicate", valid, 409, "already exists"},
		{"unknown node", `{"name":"b","engine":"ollama","model":"a","node_id":"ghost"}`, 400, "unknown node_id"},
		{"hf token without secrets", `{"name":"c","engine":"vllm","model":"o/m","hf_token":"hf_x"}`, 501, "APP_MASTER_KEY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", tt.body)
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.contains) {
				t.Errorf("status/body = %d %s, want %d containing %q", rec.Code, rec.Body.String(), tt.want, tt.contains)
			}
		})
	}
	if rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/chat/hf-token", `{"hf_token":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty hf token status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/chat/hf-token", `{"hf_token":"hf_x"}`); rec.Code != http.StatusNotImplemented {
		t.Errorf("hf token without secrets status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing model status = %d", rec.Code)
	}
}

func TestSetAppNode_GPUServicePlacement(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, id := range []string{"cpu-node", "gpu-node", "norun-node", "fresh-node"} {
		seedNode(t, db, id, id)
	}
	_ = db.SetNodeGPU(ctx, "cpu-node", gpu.Info{})
	_ = db.SetNodeGPU(ctx, "gpu-node", gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0}}})
	_ = db.SetNodeGPU(ctx, "norun-node", gpu.Info{Present: true, Devices: []gpu.Device{{Index: 0}}})

	svc := store.DesiredService{Name: "llm", Image: "img:v1", Port: 80, Resources: &store.ServiceResources{GPU: &store.ServiceGPU{Count: 1}}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "plain", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, app, node string
		want            int
	}{
		{"gpu service to cpu node rejected", "llm", "cpu-node", http.StatusBadRequest},
		{"gpu service to node without runtime rejected", "llm", "norun-node", http.StatusBadRequest},
		{"gpu service to gpu node allowed", "llm", "gpu-node", http.StatusOK},
		{"gpu service to unreported node allowed", "llm", "fresh-node", http.StatusOK},
		{"plain service unaffected", "plain", "cpu-node", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/apps/"+tt.app+"/node", `{"node_id":"`+tt.node+`"}`)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestGPUsEndpointAndDoctor(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/gpus", ""); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("no snapshots body = %s, want []", rec.Body.String())
	}

	info := gpu.Info{Present: true, DriverVersion: "550.1", Devices: []gpu.Device{{Index: 0, UUID: "GPU-a", Name: "A100", VRAMTotalMiB: 40960, VRAMUsedMiB: 1024, UtilizationPercent: 4}}}
	if err := db.SetNodeGPU(ctx, store.LocalNodeGPUKey, info); err != nil {
		t.Fatal(err)
	}
	rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/gpus", "")
	var nodes []gpuNodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &nodes); err != nil || len(nodes) != 1 {
		t.Fatalf("gpus = %s, %v", rec.Body.String(), err)
	}
	n := nodes[0]
	if !n.IsLocal || n.GPUCount != 1 || n.TotalVRAMMiB != 40960 || n.UsedVRAMMiB != 1024 || n.RuntimeInstalled || n.Hint == "" {
		t.Errorf("gpu node = %+v", n)
	}

	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/system/doctor", "")
	var doc systemDoctorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	var found *doctorCheckResource
	for i := range doc.Checks {
		if doc.Checks[i].Code == "gpu:local" {
			found = &doc.Checks[i]
		}
	}
	if found == nil || found.Status != doctorStatusWarn || found.Fix != gpu.InstallCommand {
		t.Fatalf("doctor gpu check = %+v", found)
	}

	info.RuntimeInstalled = true
	_ = db.SetNodeGPU(ctx, store.LocalNodeGPUKey, info)
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/system/doctor", "")
	doc = systemDoctorResponse{}
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	for _, c := range doc.Checks {
		if strings.HasPrefix(c.Code, "gpu:") && c.Status != doctorStatusOK {
			t.Errorf("healthy gpu check = %+v, want ok", c)
		}
	}
}

func TestModels_EngineMetricsEndpoint(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/nope/engine-metrics", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing model status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"chat","engine":"ollama","model":"llama3.1:8b"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat/engine-metrics?since=bogus", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad since status = %d", rec.Code)
	}
	rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat/engine-metrics?since=2h", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rep models.EngineMetricsReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Engine != "ollama" || rep.Collecting || rep.Health.State != models.EngineHealthUnknown || len(rep.Series) == 0 {
		t.Errorf("report = %+v", rep)
	}
}

func TestModels_FitEndpoints(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SetNodeGPU(context.Background(), store.LocalNodeGPUKey, gpu.Info{Present: true, RuntimeInstalled: true,
		Devices: []gpu.Device{{Index: 0, UUID: "GPU-a", Name: "RTX", VRAMTotalMiB: 24576}}}); err != nil {
		t.Fatal(err)
	}
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/fit", `{"engine":"nope","model":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad engine status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/fit", `{"engine":"ollama","model":"x","weights_bytes":-1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative weights status = %d", rec.Code)
	}
	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/fit", `{"engine":"ollama","model":"llama3.1:8b","context_length":8192}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rep models.FitReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Nodes) != 1 || rep.Nodes[0].Verdict != models.FitFits || rep.Note == "" || !strings.Contains(rep.Nodes[0].Arithmetic, "weights") {
		t.Fatalf("report = %+v", rep)
	}

	if rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/nope/fit", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing model status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"chat","engine":"ollama","model":"llama3.1:8b"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat/fit", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("model fit status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rep = models.FitReport{}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Model != "chat" || len(rep.Nodes) != 1 || !rep.Nodes[0].Current {
		t.Fatalf("model fit report = %+v", rep)
	}
}

func TestModels_ResidencyFlow(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"chat","engine":"ollama","model":"llama3.1:8b","residency":"on_demand","idle_ttl_seconds":600}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created createModelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Residency != "on_demand" || created.IdleTTLSeconds != 600 || created.EffectiveIdleTTLSeconds != 600 || created.LastActiveAt == nil || created.ResidencyState != "awake" {
		t.Fatalf("created = %+v", created.modelResource)
	}
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"bad","engine":"ollama","model":"llama3.1:8b","residency":"weekly"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad residency status = %d", rec.Code)
	}

	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/sleep", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("sleep status = %d", rec.Code)
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat", "")
	var got modelResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.LastActiveAt != nil {
		t.Fatalf("sleep must clear activity: %+v", got)
	}
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/wake", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("wake status = %d", rec.Code)
	}

	if rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/chat/residency", `{"residency":"always"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("set residency status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/models/chat/wake", ""},
		{http.MethodPost, "/api/v1/models/chat/sleep", ""},
	} {
		if rec := doModels(t, rt, cookie, tc.method, tc.path, tc.body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s on an always-resident model status = %d", tc.path, rec.Code)
		}
	}
	if rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/chat/residency", `{"residency":"on_demand","idle_ttl_seconds":-1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative ttl status = %d", rec.Code)
	}
	if rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/nope/residency", `{"residency":"always"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing model status = %d", rec.Code)
	}
}

func TestDoctorHostGPUChecks(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	info := gpu.Info{Present: true, DriverVersion: "550.1", RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0, UUID: "GPU-a", Name: "A100", VRAMTotalMiB: 40960, VRAMUsedMiB: 1024}}}
	if err := db.SetNodeGPU(ctx, store.LocalNodeGPUKey, info); err != nil {
		t.Fatal(err)
	}
	if err := db.SetNodeGPU(ctx, "remote", gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0, Name: "L4", VRAMTotalMiB: 24576, VRAMUsedMiB: 2048}}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.SaveNode(ctx, store.Node{ID: "remote", Name: "remote", Status: store.NodeStatusOnline, AcceptsAppWorkloads: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	rt.gpuHostDiagnoser = func(context.Context) gpu.HostDiagnosis {
		return gpu.HostDiagnosis{Attach: gpu.AttachLegacy, Findings: []gpu.Finding{
			{Code: "driver", Name: "NVIDIA driver", Status: gpu.StatusOK, Message: "driver 550.1"},
			{Code: "cdi", Name: "CDI", Status: gpu.StatusWarn, Message: "spec present, docker lists none", Fix: gpu.CDIEnableHint},
			{Code: "attach", Name: "GPU attach", Status: gpu.StatusFail, Message: "no path"},
		}}
	}
	rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/system/doctor", "")
	var doc systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	byCode := map[string]doctorCheckResource{}
	for _, c := range doc.Checks {
		byCode[c.Code] = c
	}
	if c := byCode["gpu:local:cdi"]; c.Status != doctorStatusWarn || c.Fix != gpu.CDIEnableHint || c.DocsPath == "" {
		t.Errorf("cdi check = %+v", c)
	}
	if c := byCode["gpu:local:attach"]; c.Status != doctorStatusFail {
		t.Errorf("attach check = %+v", c)
	}
	if doc.OK {
		t.Error("a failing GPU attach check must fail the doctor report")
	}
	if _, ok := byCode["gpu:remote:cdi"]; ok {
		t.Error("remote nodes are not inspected for CDI")
	}
	if c := byCode["gpu:remote:gpu0"]; c.Status != doctorStatusOK || !strings.Contains(c.Message, "L4") {
		t.Errorf("remote per-GPU memory = %+v", c)
	}
}
