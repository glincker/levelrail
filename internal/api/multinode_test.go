package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// runningAppRuntime reports one running container for "web", so Ready can pass.
type runningAppRuntime struct{ fakeMoveVolumeRuntime }

func (f *runningAppRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return []docker.ContainerState{{ID: "c1", Name: application.ContainerName("web", "img:1", ""), Running: true}}, nil
}

type recordingCloser struct{ closed []string }

func (r *recordingCloser) Disconnect(id string) { r.closed = append(r.closed, id) }

func newMultiNodeRouter(t *testing.T, opts ...Option) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	fake := &runningAppRuntime{}
	base := []Option{
		WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }),
		WithMultiNode(db, nodemove.Config{VerifyTimeout: time.Minute, MaxAttempts: 3, Interval: time.Second}, 21000, 21999),
	}
	rt := NewRouter(discardLogger(), testBrand(), db, append(base, opts...)...)
	return rt, db, loginTestSession(t, rt, db)
}

func seedMNNode(t *testing.T, db *store.DB, id string, labels map[string]string) {
	t.Helper()
	seedNode(t, db, id, id)
	if err := db.UpdateNodeStatus(context.Background(), id, store.NodeStatusOnline); err != nil {
		t.Fatal(err)
	}
	if labels != nil {
		if err := db.SetNodeLabels(context.Background(), id, labels); err != nil {
			t.Fatal(err)
		}
	}
}

func mnDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestMultiNode_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/node-moves", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestNodeLabels(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", nil)
	tests := []struct {
		body string
		want int
	}{
		{`{"labels":{"disk":"ssd","zone":"eu-1"}}`, http.StatusOK},
		{`{"labels":{"Disk":"ssd"}}`, http.StatusBadRequest},
		{`{"labels":{"disk":"has space"}}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		if rec := mnDo(t, rt, cookie, http.MethodPut, "/api/v1/nodes/n1/labels", tt.body); rec.Code != tt.want {
			t.Fatalf("PUT %s = %d, want %d: %s", tt.body, rec.Code, tt.want, rec.Body.String())
		}
	}
	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/n1/labels", "")
	if !strings.Contains(rec.Body.String(), `"zone":"eu-1"`) {
		t.Fatalf("GET labels = %s", rec.Body.String())
	}
	if rec := mnDo(t, rt, cookie, http.MethodPut, "/api/v1/nodes/ghost/labels", `{"labels":{}}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown node = %d, want 404", rec.Code)
	}
}

func TestListZones(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", map[string]string{"zone": "a", "region": "eu", "provider": "hetzner"})
	seedMNNode(t, db, "n2", map[string]string{"zone": "b"})
	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/zones", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got zonesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Labeled || len(got.Zones) != 2 || len(got.Nodes) != 2 || got.Zones[0].Region != "eu" {
		t.Fatalf("zones = %+v", got)
	}
}

func seedMNApp(t *testing.T, db *store.DB, node string) {
	const name = "web"
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "img:1"}); err != nil {
		t.Fatal(err)
	}
	if node != "" {
		if err := db.UpdateServiceNode(context.Background(), name, node); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlacementSelector(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", map[string]string{"disk": "ssd"})
	seedMNNode(t, db, "n2", nil)
	seedMNApp(t, db, "")
	rec := mnDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/placement", `{"node_selector":{"disk":"ssd"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT placement = %d: %s", rec.Code, rec.Body.String())
	}
	var view placementResource
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Eligible) != 1 || view.Eligible[0] != "n1" || len(view.Rejected) != 1 {
		t.Fatalf("view = %+v", view)
	}
	if rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/move", `{"node_id":"n2"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "labels do not match") {
		t.Fatalf("move to non-matching node = %d %s, want 400", rec.Code, rec.Body.String())
	}
	rec = mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/move", `{}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"to_node_id":"n1"`) {
		t.Fatalf("auto move = %d %s, want 202 to n1", rec.Code, rec.Body.String())
	}
}

func TestStartMove_Validation(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "cordoned", nil)
	if err := db.SetNodeSchedulable(context.Background(), "cordoned", false); err != nil {
		t.Fatal(err)
	}
	seedMNApp(t, db, "n1")
	tests := []struct {
		name, path, body string
		want             int
	}{
		{"unknown app", "/api/v1/apps/ghost/move", `{"node_id":"n1"}`, http.StatusNotFound},
		{"same node", "/api/v1/apps/web/move", `{"node_id":"n1"}`, http.StatusBadRequest},
		{"cordoned target", "/api/v1/apps/web/move", `{"node_id":"cordoned"}`, http.StatusBadRequest},
		{"unknown target", "/api/v1/apps/web/move", `{"node_id":"nope"}`, http.StatusBadRequest},
		{"to the local node", "/api/v1/apps/web/move", `{"node_id":""}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := mnDo(t, rt, cookie, http.MethodPost, tt.path, tt.body); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestStartMove_RunsToCompletion(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", nil)
	seedMNApp(t, db, "")
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/move", `{"node_id":"n1"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	var m nodeMoveResource
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if again := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/move", `{"node_id":"n1"}`); again.Code != http.StatusConflict {
		t.Fatalf("second start = %d, want 409", again.Code)
	}

	ctx := context.Background()
	got, err := rt.multiNode.runner.Advance(ctx, m.ID)
	if err != nil || got.Phase != nodemove.PhaseVerify {
		t.Fatalf("after first advance phase = %s err = %v, want verify (no fresh Ready yet)", got.Phase, err)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); svc.NodeID != "n1" {
		t.Fatalf("placement = %q, want staged on n1", svc.NodeID)
	}
	if err := db.UpsertConditions(ctx, "application/web", []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: "Running"}}); err != nil {
		t.Fatal(err)
	}
	got, err = rt.multiNode.runner.Advance(ctx, m.ID)
	if err != nil || got.Status != store.NodeMoveStatusSucceeded {
		t.Fatalf("final = %s/%s err=%v (%s) steps=%+v placed=%s", got.Status, got.Phase, err, got.Error, got.Steps, got.PlacedAt)
	}
	rec = mnDo(t, rt, cookie, http.MethodGet, "/api/v1/node-moves/"+m.ID, "")
	if !strings.Contains(rec.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("GET move = %s", rec.Body.String())
	}
}

func TestStartMove_RefusesBindMountsAndCrossNodeDatabaseWithoutMesh(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	ctx := context.Background()
	seedMNNode(t, db, "n1", nil)
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "api", Image: "img:1",
		DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}}}); err != nil {
		t.Fatal(err)
	}
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/api/move", `{"node_id":"n1"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	var m nodeMoveResource
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	got, _ := rt.multiNode.runner.Advance(ctx, m.ID)
	if got.Status != store.NodeMoveStatusFailed || !strings.Contains(got.Error, "needs mesh networking") {
		t.Fatalf("move = %s %q, want refused for a cross-node database without mesh", got.Status, got.Error)
	}
	if svc, _ := db.GetDesiredService(ctx, "api"); svc.NodeID != "" {
		t.Fatalf("refused move changed placement to %q", svc.NodeID)
	}
}

func TestDrain_StartsJobAndCordons(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "n2", nil)
	seedMNApp(t, db, "n1")
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/n1/drain", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("drain = %d %s", rec.Code, rec.Body.String())
	}
	n, _ := db.GetNode(context.Background(), "n1")
	if n.Schedulable {
		t.Fatal("drain did not cordon the node")
	}
	if again := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/n1/drain", ""); again.Code != http.StatusConflict {
		t.Fatalf("second drain = %d, want 409", again.Code)
	}
	rt.multiNode.runner.Tick(context.Background())
	rec = mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/n1/drains", "")
	if !strings.Contains(rec.Body.String(), `"resource":"web"`) || !strings.Contains(rec.Body.String(), `"to_node_id":"n2"`) {
		t.Fatalf("drain moves = %s", rec.Body.String())
	}
	if bad := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/n2/drain?target_node_id=n1", ""); bad.Code != http.StatusBadRequest {
		t.Fatalf("drain onto a cordoned node = %d, want 400", bad.Code)
	}
}

func TestJoinToken_CommandsAndTTL(t *testing.T) {
	rt, _, cookie := newMultiNodeRouter(t, WithNodeJoinTokenTTL(3*time.Minute))
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", `{"control_plane_addr":"cp.example.com:9443","node_name":"w2","mesh":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var resp createNodeJoinTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.JoinCommands == nil || !strings.Contains(resp.JoinCommands.Docker, "APP_JOIN_TOKEN="+resp.Token) ||
		!strings.Contains(resp.JoinCommands.Docker, "NET_ADMIN") || !strings.Contains(resp.JoinCommands.Binary, "APP_NODE_NAME=w2") {
		t.Fatalf("commands = %+v", resp.JoinCommands)
	}
	if p := resp.JoinCommands.Podman; !strings.Contains(p, "podman run") || !strings.Contains(p, "/run/podman/podman.sock") ||
		!strings.Contains(p, "APP_CONTROL_PLANE_ADDR=cp.example.com:9443") || !strings.Contains(p, "NET_ADMIN") {
		t.Fatalf("podman command = %q", p)
	}
	if ttl := time.Until(resp.ExpiresAt); ttl > 3*time.Minute || ttl < 2*time.Minute {
		t.Fatalf("ttl = %s, want about 3m", ttl)
	}
	for _, body := range []string{`{"control_plane_addr":"cp; rm -rf /"}`, `{"control_plane_addr":"cp:1","node_name":"Bad Name"}`} {
		if bad := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", body); bad.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", body, bad.Code)
		}
	}
}

func TestClampJoinTokenTTL(t *testing.T) {
	tests := []struct{ in, want time.Duration }{
		{0, nodeJoinTokenTTL}, {time.Second, time.Minute}, {time.Hour, time.Hour}, {72 * time.Hour, 24 * time.Hour},
	}
	for _, tt := range tests {
		if got := clampJoinTokenTTL(tt.in); got != tt.want {
			t.Errorf("clamp(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestDeleteNode_ClosesLiveSession(t *testing.T) {
	closer := &recordingCloser{}
	rt, db, cookie := newMultiNodeRouter(t, WithNodeSessionCloser(closer))
	seedMNNode(t, db, "n1", nil)
	if rec := mnDo(t, rt, cookie, http.MethodDelete, "/api/v1/nodes/n1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if len(closer.closed) != 1 || closer.closed[0] != "n1" {
		t.Fatalf("closed = %v, want the deleted node's session closed", closer.closed)
	}
}

func TestPairStatus(t *testing.T) {
	h, s, n, u, np := meshLinkView{State: linkHealthy}, meshLinkView{State: linkStale}, meshLinkView{State: linkNever}, meshLinkView{State: linkUnknown}, meshLinkView{State: linkNoPeer}
	tests := []struct {
		ab, ba meshLinkView
		want   string
	}{
		{h, h, pairOK}, {h, s, pairDegraded}, {n, s, pairDown}, {u, u, pairUnknown}, {np, h, pairDown}, {u, h, pairDegraded},
	}
	for _, tt := range tests {
		if got, _ := pairStatus(tt.ab, tt.ba); got != tt.want {
			t.Errorf("pairStatus(%s, %s) = %s, want %s", tt.ab.State, tt.ba.State, got, tt.want)
		}
	}
}

func TestGetNode_AcceptsNameAndListsLabels(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedNode(t, db, "n_b", "worker-b")
	if err := db.SetNodeLabels(context.Background(), "n_b", map[string]string{"disk": "ssd"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"n_b", "worker-b"} {
		rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/"+key, "")
		var got nodeResource
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("GET %s = %d %s", key, rec.Code, rec.Body.String())
		}
		if got.ID != "n_b" || got.Labels["disk"] != "ssd" {
			t.Fatalf("node = %+v", got)
		}
	}
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/no-such", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", rec.Code)
	}
}

func TestToNodeResource_FallsBackToMeshAddressAndEnrollTime(t *testing.T) {
	created := time.Now().Add(-time.Hour)
	res := toNodeResource(store.Node{ID: "x", Status: store.NodeStatusOnline, MeshAddress: "10.77.0.3", CreatedAt: created})
	if res.Address != "10.77.0.3" || res.JoinedAt == nil || !res.JoinedAt.Equal(created) {
		t.Fatalf("address=%q joined=%v", res.Address, res.JoinedAt)
	}
	pending := toNodeResource(store.Node{ID: "y", Status: store.NodeStatusPending, CreatedAt: created})
	if pending.JoinedAt != nil {
		t.Fatal("a pending node has not joined yet")
	}
}
