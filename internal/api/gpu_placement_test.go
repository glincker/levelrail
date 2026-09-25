package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

func gpuInfoN(n int, runtime bool) gpu.Info {
	info := gpu.Info{Present: n > 0, RuntimeInstalled: runtime}
	for i := 0; i < n; i++ {
		info.Devices = append(info.Devices, gpu.Device{Index: i, VRAMTotalMiB: 24576, VRAMUsedMiB: 1024})
	}
	return info
}

func seedGPUNode(t *testing.T, db *store.DB, id string, info gpu.Info) {
	t.Helper()
	seedOnlineNode(t, db, id, id, true)
	if err := db.SetNodeGPU(context.Background(), id, info); err != nil {
		t.Fatalf("set node gpu: %v", err)
	}
}

func seedGPUApp(t *testing.T, db *store.DB, name, nodeID string, g store.ServiceGPU) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:1", Port: 3000, Resources: &store.ServiceResources{GPU: &g}}); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	if err := db.UpdateServiceNode(ctx, name, nodeID); err != nil {
		t.Fatalf("place %s: %v", name, err)
	}
}

func drain(t *testing.T, rt *Router, cookie *http.Cookie, target, query string) (int, drainNodeResponse) {
	t.Helper()
	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/nodes/"+target+"/drain"+query, "")
	var got drainNodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode drain: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, got
}

func nodeOf(t *testing.T, db *store.DB, name string) string {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), name)
	if err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	return svc.NodeID
}

func TestDrain_GPUAware(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, db *store.DB)
		query       string
		wantCode    int
		wantMoved   map[string]string
		wantBlocked []string
	}{
		{
			name: "moves to the node with a free GPU, skipping a full one",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(1, true))
				seedGPUNode(t, db, "full", gpuInfoN(1, true))
				seedGPUNode(t, db, "free", gpuInfoN(2, true))
				seedGPUApp(t, db, "job", "src", store.ServiceGPU{Count: 1})
				seedGPUApp(t, db, "busy", "full", store.ServiceGPU{Count: 1})
			},
			wantCode:  http.StatusOK,
			wantMoved: map[string]string{"job": "free"},
		},
		{
			name: "exactly full node blocks the app and it stays put",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(1, true))
				seedGPUNode(t, db, "full", gpuInfoN(1, true))
				seedGPUApp(t, db, "job", "src", store.ServiceGPU{Count: 1})
				seedGPUApp(t, db, "busy", "full", store.ServiceGPU{Count: 1})
			},
			wantCode:    http.StatusMultiStatus,
			wantMoved:   map[string]string{"job": "src"},
			wantBlocked: []string{"app/job"},
		},
		{
			name: "runtime missing on the only other node",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(1, true))
				seedGPUNode(t, db, "norun", gpuInfoN(1, false))
				seedGPUApp(t, db, "job", "src", store.ServiceGPU{Count: 1})
			},
			wantCode:    http.StatusMultiStatus,
			wantMoved:   map[string]string{"job": "src"},
			wantBlocked: []string{"app/job"},
		},
		{
			name: "device id overlap blocks",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(2, true))
				seedGPUNode(t, db, "dst", gpuInfoN(2, true))
				seedGPUApp(t, db, "job", "src", store.ServiceGPU{DeviceIDs: []string{"0"}})
				seedGPUApp(t, db, "pinned", "dst", store.ServiceGPU{DeviceIDs: []string{"0"}})
			},
			wantCode:    http.StatusMultiStatus,
			wantMoved:   map[string]string{"job": "src"},
			wantBlocked: []string{"app/job"},
		},
		{
			name: "two apps compete for one free GPU",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(2, true))
				seedGPUNode(t, db, "dst", gpuInfoN(1, true))
				seedGPUApp(t, db, "a", "src", store.ServiceGPU{Count: 1})
				seedGPUApp(t, db, "b", "src", store.ServiceGPU{Count: 1})
			},
			wantCode:    http.StatusMultiStatus,
			wantMoved:   map[string]string{"a": "dst", "b": "src"},
			wantBlocked: []string{"app/b"},
		},
		{
			name: "explicit target without a GPU blocks",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(1, true))
				seedGPUNode(t, db, "cpu", gpu.Info{})
				seedGPUApp(t, db, "job", "src", store.ServiceGPU{Count: 1})
			},
			query:       "?target_node_id=cpu",
			wantCode:    http.StatusMultiStatus,
			wantMoved:   map[string]string{"job": "src"},
			wantBlocked: []string{"app/job"},
		},
		{
			name: "cpu app still moves freely",
			setup: func(t *testing.T, db *store.DB) {
				seedGPUNode(t, db, "src", gpuInfoN(1, true))
				seedGPUNode(t, db, "cpu", gpu.Info{})
				if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
					t.Fatal(err)
				}
				if err := db.UpdateServiceNode(context.Background(), "web", "src"); err != nil {
					t.Fatal(err)
				}
			},
			wantCode:  http.StatusOK,
			wantMoved: map[string]string{"web": "cpu"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newModelsTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			tt.setup(t, db)

			code, got := drain(t, rt, cookie, "src", tt.query)
			if code != tt.wantCode {
				t.Fatalf("status = %d, want %d (%+v)", code, tt.wantCode, got)
			}
			for name, node := range tt.wantMoved {
				if n := nodeOf(t, db, name); n != node {
					t.Errorf("%s on %q, want %q", name, n, node)
				}
			}
			var blocked []string
			for _, b := range got.Blocked {
				blocked = append(blocked, b.Kind+"/"+b.Name)
				if b.Reason == "" || !strings.Contains(b.Reason, "no GPU node available") {
					t.Errorf("blocked reason = %q", b.Reason)
				}
			}
			if strings.Join(blocked, ",") != strings.Join(tt.wantBlocked, ",") {
				t.Errorf("blocked = %v, want %v", blocked, tt.wantBlocked)
			}
		})
	}
}

func TestDrain_ReportsPinnedModels(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedGPUNode(t, db, "src", gpuInfoN(1, true))
	if err := db.SaveModel(context.Background(), store.Model{Name: "chat", Engine: "ollama", ModelRef: "m", NodeID: "src", GPUCount: 1}); err != nil {
		t.Fatal(err)
	}
	code, got := drain(t, rt, cookie, "src", "")
	if code != http.StatusMultiStatus || len(got.Blocked) != 1 || got.Blocked[0].Kind != "model" || got.Blocked[0].Name != "chat" {
		t.Fatalf("code=%d blocked=%+v", code, got.Blocked)
	}
}

func TestCreateApp_GPUAutoPlacement(t *testing.T) {
	body := `{"name":"llm","image":"img:1","port":8000,"resources":{"gpu":{"count":1}}}`
	tests := []struct {
		name     string
		setup    func(t *testing.T, db *store.DB)
		wantCode int
		wantNode string
	}{
		{"skips cpu and runtime-less nodes", func(t *testing.T, db *store.DB) {
			seedGPUNode(t, db, "a-cpu", gpu.Info{})
			seedGPUNode(t, db, "b-norun", gpuInfoN(1, false))
			seedGPUNode(t, db, "c-gpu", gpuInfoN(1, true))
		}, http.StatusCreated, "c-gpu"},
		{"skips exactly full node", func(t *testing.T, db *store.DB) {
			seedGPUNode(t, db, "a-full", gpuInfoN(1, true))
			seedGPUNode(t, db, "b-free", gpuInfoN(1, true))
			seedGPUApp(t, db, "busy", "a-full", store.ServiceGPU{Count: 1})
		}, http.StatusCreated, "b-free"},
		{"no GPU node anywhere is refused", func(t *testing.T, db *store.DB) {
			seedGPUNode(t, db, "a-cpu", gpu.Info{})
			seedGPUNode(t, db, "b-norun", gpuInfoN(1, false))
		}, http.StatusConflict, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newModelsTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			tt.setup(t, db)
			rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/apps", body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode == http.StatusCreated {
				if n := nodeOf(t, db, "llm"); n != tt.wantNode {
					t.Errorf("placed on %q, want %q", n, tt.wantNode)
				}
			}
		})
	}
}

func TestSetAppNode_GPUExactlyFull(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedGPUNode(t, db, "full", gpuInfoN(1, true))
	seedGPUNode(t, db, "src", gpuInfoN(1, true))
	seedGPUApp(t, db, "busy", "full", store.ServiceGPU{Count: 1})
	seedGPUApp(t, db, "job", "src", store.ServiceGPU{Count: 1})

	rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/apps/job/node", `{"node_id":"full"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not enough free GPUs") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestGPUSurfaces_ReservationsAndAttention(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedGPUNode(t, db, "n1", gpuInfoN(2, true))
	seedGPUApp(t, db, "job", "n1", store.ServiceGPU{Count: 2})
	seedGPUNode(t, db, "n2", gpuInfoN(1, false))
	seedGPUApp(t, db, "stuck", "n2", store.ServiceGPU{Count: 1})

	rec := doModels(t, rt, cookie, http.MethodGet, "/api/v1/gpus", "")
	var gpus []gpuNodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &gpus); err != nil {
		t.Fatal(err)
	}
	byName := map[string]gpuNodeResource{}
	for _, g := range gpus {
		byName[g.Name] = g
	}
	if g := byName["n1"]; g.ReservedGPUs != 2 || g.FreeGPUs != 0 || len(g.Reservations) != 1 || g.Reservations[0] != "app:job" {
		t.Errorf("n1 = %+v", g)
	}

	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/nodes", "")
	var nodes []nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &nodes); err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.ID == "n1" && (n.GPU == nil || n.GPU.GPUCount != 2 || n.GPU.ReservedGPUs != 2 || n.GPU.FreeGPUs != 0) {
			t.Errorf("n1 node gpu = %+v", n.GPU)
		}
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/nodes/n1", "")
	var one nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil || one.GPU == nil || one.GPU.TotalVRAMMiB != 2*24576 {
		t.Errorf("node detail gpu = %+v err=%v", one.GPU, err)
	}

	checks := rt.doctorCheckGPUPlacement(context.Background())
	if len(checks) != 1 || checks[0].Code != "gpu-placement:app:stuck" || checks[0].Status != doctorStatusWarn || checks[0].Fix == "" {
		t.Fatalf("checks = %+v", checks)
	}
}
