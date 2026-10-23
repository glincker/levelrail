package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dindEndpoints plays the WireGuard mesh: each node's mesh address is its
// daemon's IP on the shared test network.
type dindEndpoints map[string]string

func (d dindEndpoints) Endpoint(_ context.Context, nodeID string) (meshpath.Path, error) {
	ip, ok := d[nodeID]
	if !ok {
		return meshpath.Path{Reason: "unknown node"}, nil
	}
	return meshpath.Path{Usable: true, Address: ip}, nil
}

type fixedSecret string

func (s fixedSecret) Exists(_ context.Context, _ string, key string) (bool, error) {
	return key != database.TLSCertEnvKey, nil
}
func (s fixedSecret) Resolve(context.Context, string, string) (string, error) {
	return string(s), nil
}

const (
	livePGImage     = "postgres:16-alpine"
	liveHelperImage = "alpine:3.20"
	liveZone        = "lrtest"
	livePassword    = "live-multinode-pw" //nolint:gosec // throwaway test database password
)

// TestMultiNode_Live_MoveAndDrainKeepDatabaseConnection is the Phase 3 exit
// check against two real Docker daemons and two enrolled agents: an app
// keeps reaching its database by name after moving away from it, and a
// drain moves the database (with its data) and empties the node.
func TestMultiNode_Live_MoveAndDrainKeepDatabaseConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("live multi-node test skipped under -short")
	}
	host := liveHostDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	daemons := startDindNodes(ctx, t, host, 2, livePGImage, liveHelperImage)

	db := openTestDB(t)
	registry := agent.NewRegistry()
	onboarding := agent.NewOnboardingStore()
	addr := startLiveAgentServer(t, db, registry, onboarding)
	cfg := nodemove.Config{VerifyTimeout: 4 * time.Minute, MaxAttempts: 5, Interval: 500 * time.Millisecond}
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithExecRuntime(func(id string) (docker.Runtime, error) { return registry.Get(id) }),
		WithMultiNode(db, cfg, 21000, 21099))
	rt.SetLocalNodeID("live-control-plane")
	rt.SetNodeOnboarding(onboarding, nodepreflight.ConfigFromEnv(func(string) (string, bool) { return "", false }))
	cookie := loginTestSession(t, rt, db)

	nodeIDs := make([]string, len(daemons))
	endpoints := dindEndpoints{}
	for i, d := range daemons {
		nodeIDs[i] = enrollLiveAgent(ctx, t, rt, cookie, addr, registry, d, fmt.Sprintf("worker-%d", i))
		endpoints[nodeIDs[i]] = d.meshIP
	}
	nodeA, nodeB := nodeIDs[0], nodeIDs[1]
	assertLiveOnboarding(ctx, t, rt, db, cookie, nodeIDs)

	seedLiveWorkload(ctx, t, db, nodeA)
	go runLiveReconcileLoop(ctx, db, registry, endpoints)
	go rt.RunNodeMoves(ctx)

	waitLive(t, 4*time.Minute, "app ready on node A", func() (bool, string) { return liveReady(ctx, db, "application/web") })
	waitLive(t, 2*time.Minute, "seed a row through DATABASE_URL", func() (bool, string) {
		out, err := psqlInApp(ctx, registry, nodeA, "CREATE TABLE IF NOT EXISTS t (v text); DELETE FROM t; INSERT INTO t VALUES ('kept-across-nodes');")
		return err == nil, fmt.Sprint(out, err)
	})

	move := startLiveMove(t, rt, cookie, "/api/v1/apps/web/move", nodeB)
	final := waitLiveMove(ctx, t, db, move)
	if final.Status != store.NodeMoveStatusSucceeded {
		t.Fatalf("app move = %s: %s (steps %+v)", final.Status, final.Error, final.Steps)
	}
	assertNoContainer(ctx, t, registry, nodeA, "web-")
	waitLive(t, 2*time.Minute, "app on node B reads its database on node A by name", func() (bool, string) {
		out, err := psqlInApp(ctx, registry, nodeB, "SELECT v FROM t;")
		return err == nil && strings.Contains(out, "kept-across-nodes"), fmt.Sprint(out, err)
	})
	env := appEnv(ctx, t, registry, nodeB)
	if !strings.Contains(env, "@main."+liveZone+":21000/") {
		t.Fatalf("cross-node DATABASE_URL does not use the mesh name and port: %s", env)
	}

	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/"+nodeA+"/drain", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("drain = %d %s", rec.Code, rec.Body.String())
	}
	var drain nodeDrainResource
	_ = json.Unmarshal(rec.Body.Bytes(), &drain)
	waitLive(t, 8*time.Minute, "drain of node A", func() (bool, string) {
		d, err := db.GetNodeDrain(ctx, drain.ID)
		return err == nil && d.Status != store.NodeMoveStatusRunning, fmt.Sprintf("%+v %v", d, err)
	})
	if d, _ := db.GetNodeDrain(ctx, drain.ID); d.Status != store.NodeMoveStatusSucceeded {
		moves, _ := db.ListNodeMoves(ctx, store.NodeMoveFilter{DrainID: drain.ID})
		t.Fatalf("drain = %s: %s; moves %+v", d.Status, d.Error, moves)
	}
	assertNoContainer(ctx, t, registry, nodeA, "web-")
	assertNoContainer(ctx, t, registry, nodeA, database.ContainerName("main"))
	waitLive(t, 3*time.Minute, "app reads the copied database after the drain", func() (bool, string) {
		out, err := psqlInApp(ctx, registry, nodeB, "SELECT v FROM t;")
		return err == nil && strings.Contains(out, "kept-across-nodes"), fmt.Sprint(out, err)
	})
}

// assertLiveOnboarding checks the preflight a real agent reports about a
// real Docker daemon, and that its manual labels reach the node row.
func assertLiveOnboarding(ctx context.Context, t *testing.T, rt *Router, db *store.DB, cookie *http.Cookie, nodeIDs []string) {
	t.Helper()
	for i, id := range nodeIDs {
		var ob nodeOnboardingResponse
		waitLive(t, time.Minute, "onboarding report from "+id, func() (bool, string) {
			rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/"+id+"/onboarding", "")
			if err := json.Unmarshal(rec.Body.Bytes(), &ob); err != nil {
				return false, err.Error()
			}
			return ob.Reported, rec.Body.String()
		})
		for _, c := range ob.Checks {
			if c.ID == nodepreflight.CheckDocker && c.Status != nodepreflight.StatusPass {
				t.Errorf("node %d docker check = %+v against a live daemon", i, c)
			}
		}
		if ob.Detected == nil || ob.Detected.Provider != "livelab" || ob.Detected.Source != cloudmeta.SourceEnv {
			t.Errorf("node %d detected = %+v", i, ob.Detected)
		}
		rt.syncDetectedDomains(ctx)
		n, err := db.GetNode(ctx, id)
		if err != nil || n.Provider != "livelab" || n.Zone != fmt.Sprintf("lab-%d", i) {
			t.Errorf("node %d row = %+v err=%v", i, n, err)
		}
	}
}

func startLiveAgentServer(t *testing.T, db *store.DB, registry *agent.Registry, onboarding *agent.OnboardingStore) string {
	t.Helper()
	ca, err := agent.GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	creds, err := agent.NewServerCredentials(ca, []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(creds))
	agentpb.RegisterAgentServiceServer(srv, agent.NewServer(ca, db, registry, nil, agent.WithOnboardingSink(onboarding)))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// enrollLiveAgent mints a join token through the API, enrolls an agent
// that talks to d's daemon, persists its identity in its own data dir and
// starts its session.
func enrollLiveAgent(ctx context.Context, t *testing.T, rt *Router, cookie *http.Cookie, addr string, registry *agent.Registry, d dindNode, name string) string {
	t.Helper()
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", `{"control_plane_addr":"`+addr+`"}`)
	var tok createNodeJoinTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tok); err != nil || tok.Token == "" {
		t.Fatalf("join token: %d %s", rec.Code, rec.Body.String())
	}
	id, err := agent.DialEnroll(ctx, addr, tok.Token, name)
	if err != nil {
		t.Fatalf("enroll %s: %v", name, err)
	}
	if _, err := agent.DialEnroll(ctx, addr, tok.Token, name+"-again"); err == nil {
		t.Fatal("a join token enrolled twice")
	}
	if err := agent.NewIdentityFile(t.TempDir() + "/identity.json").Save(id); err != nil {
		t.Fatalf("save identity: %v", err)
	}
	client, err := docker.NewClient(docker.WithHost(d.host))
	if err != nil {
		t.Fatal(err)
	}
	sctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	cloud := cloudmeta.Config{Disabled: true, Override: cloudmeta.Info{Provider: "livelab", Region: "lab", Zone: strings.Replace(name, "worker-", "lab-", 1)}}
	probe := agent.NewOnboardingProbe(client, t.TempDir(), false, cloud)
	go func() {
		_ = agent.RunSession(sctx, addr, id, client, nil, agent.WithOnboardingProbe(probe, 200*time.Millisecond))
	}()
	waitLive(t, 30*time.Second, name+" session", func() (bool, string) {
		_, err := registry.Get(id.NodeID)
		return err == nil, fmt.Sprint(err)
	})
	if err := rt.nodes.(*store.DB).UpdateNodeStatus(ctx, id.NodeID, store.NodeStatusOnline); err != nil {
		t.Fatal(err)
	}
	return id.NodeID
}

func seedLiveWorkload(ctx context.Context, t *testing.T, db *store.DB, node string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16-alpine"}))
	must(db.UpdateDatabaseNode(ctx, "main", node))
	must(db.SaveApp(ctx, store.App{ID: "app_live", Name: "live"}))
	must(db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: livePGImage, AppID: "app_live", Command: []string{"sleep", "100000"},
		DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}},
	}))
	must(db.UpdateServiceNode(ctx, "web", node))
}

// runLiveReconcileLoop is the engine's job reduced to what this test needs:
// reconcile every app and database through its node's agent and record conditions.
func runLiveReconcileLoop(ctx context.Context, db *store.DB, registry *agent.Registry, endpoints dindEndpoints) {
	mesh := application.MeshDatabaseConfig{Endpoints: endpoints, Ports: db, Zone: liveZone, LocalNodeID: "live-control-plane", MinPort: 21000, MaxPort: 21099}
	for ctx.Err() == nil {
		dbs, _ := db.ListDesiredDatabases(ctx)
		for _, d := range dbs {
			if rt, err := registry.Get(d.NodeID); err == nil {
				c := database.New(d.Name, db, rt, database.WithMeshEndpoint(endpoints, db),
					database.WithPostgresCredentials(&database.PostgresCredentials{Username: d.Name, Password: livePassword}))
				res, _ := c.Reconcile(ctx)
				_ = db.UpsertConditions(ctx, c.Name(), res.Conditions)
			}
		}
		svcs, _ := db.ListDesiredServices(ctx)
		for _, s := range svcs {
			if rt, err := registry.Get(s.NodeID); err == nil {
				c := application.New(s.Name, db, rt, application.WithDatabaseAttachments(db),
					application.WithSecretResolver(fixedSecret(livePassword)), application.WithMeshDatabases(mesh))
				res, _ := c.Reconcile(ctx)
				_ = db.UpsertConditions(ctx, c.Name(), res.Conditions)
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func liveReady(ctx context.Context, db *store.DB, controller string) (bool, string) {
	conds, err := db.GetConditions(ctx, controller)
	for _, c := range conds {
		if c.Type == "Ready" {
			return c.Status == "True", c.Reason + ": " + c.Message
		}
	}
	return false, fmt.Sprint("no conditions yet ", err)
}

func appContainer(ctx context.Context, registry *agent.Registry, node string) (docker.Runtime, string, error) {
	rt, err := registry.Get(node)
	if err != nil {
		return nil, "", err
	}
	cs, err := rt.ListByPrefix(ctx, "web-")
	if err != nil {
		return nil, "", err
	}
	for _, c := range cs {
		if svc, ok := application.ParseContainerName(c.Name); ok && svc == "web" && c.Running {
			return rt, c.ID, nil
		}
	}
	return nil, "", fmt.Errorf("no running web container on %s", node)
}

func psqlInApp(ctx context.Context, registry *agent.Registry, node, sql string) (string, error) {
	rt, id, err := appContainer(ctx, registry, node)
	if err != nil {
		return "", err
	}
	execer, ok := rt.(interface {
		Exec(context.Context, string, []string) (io.ReadCloser, error)
	})
	if !ok {
		return "", fmt.Errorf("runtime cannot exec")
	}
	rc, err := execer.Exec(ctx, id, []string{"sh", "-c", `psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -tAc "` + sql + `" 2>&1`})
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if strings.Contains(string(out), "ERROR") || strings.Contains(string(out), "could not") {
		return string(out), fmt.Errorf("psql failed")
	}
	return string(out), err
}

func appEnv(ctx context.Context, t *testing.T, registry *agent.Registry, node string) string {
	t.Helper()
	rt, id, err := appContainer(ctx, registry, node)
	if err != nil {
		t.Fatal(err)
	}
	execer := rt.(interface {
		Exec(context.Context, string, []string) (io.ReadCloser, error)
	})
	rc, err := execer.Exec(ctx, id, []string{"sh", "-c", "echo $DATABASE_URL; cat /etc/hosts"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	out, _ := io.ReadAll(rc)
	return string(out)
}

func assertNoContainer(ctx context.Context, t *testing.T, registry *agent.Registry, node, prefix string) {
	t.Helper()
	rt, err := registry.Get(node)
	if err != nil {
		t.Fatal(err)
	}
	waitLive(t, time.Minute, "no "+prefix+" left on "+node, func() (bool, string) {
		cs, err := rt.ListByPrefix(ctx, prefix)
		return err == nil && len(cs) == 0, fmt.Sprintf("%d containers, %v", len(cs), err)
	})
}

func startLiveMove(t *testing.T, rt *Router, cookie *http.Cookie, path, node string) nodeMoveResource {
	t.Helper()
	rec := mnDo(t, rt, cookie, http.MethodPost, path, `{"node_id":"`+node+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST %s = %d %s", path, rec.Code, rec.Body.String())
	}
	var m nodeMoveResource
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return m
}

func waitLiveMove(ctx context.Context, t *testing.T, db *store.DB, m nodeMoveResource) store.NodeMove {
	t.Helper()
	var got store.NodeMove
	waitLive(t, 6*time.Minute, "move "+m.ID, func() (bool, string) {
		var err error
		got, err = db.GetNodeMove(ctx, m.ID)
		return err == nil && got.Status != store.NodeMoveStatusRunning, got.Phase
	})
	return got
}

func waitLive(t *testing.T, timeout time.Duration, what string, check func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		ok, detail := check()
		if ok {
			return
		}
		last = detail
		time.Sleep(time.Second)
	}
	t.Fatalf("timed out waiting for %s: %s", what, last)
}
