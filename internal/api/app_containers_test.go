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
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appContainersRuntime adds the optional exit-state and stats capabilities
// the local Docker client and the agent transport both implement.
type appContainersRuntime struct {
	*fakeExecAppRuntime
	exit  *docker.ExitState
	stats docker.ContainerStats
}

func (f *appContainersRuntime) InspectExitState(context.Context, string) (*docker.ExitState, error) {
	return f.exit, nil
}

func (f *appContainersRuntime) Stats(context.Context, string) (docker.ContainerStats, error) {
	return f.stats, nil
}

func seedContainersApp(t *testing.T, db *store.DB, nodeID string) string {
	t.Helper()
	if nodeID != "" {
		if err := db.SaveNode(context.Background(), store.Node{ID: nodeID, Name: nodeID, Status: store.NodeStatusOnline, Schedulable: true}); err != nil {
			t.Fatal(err)
		}
	}
	svc := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 8080, NodeID: nodeID, Replicas: 1,
		Env:       map[string]string{"DB_PASSWORD": "hunter2-super-secret"},
		Volumes:   []store.ServiceVolume{{Name: "web-data", ContainerPath: "/data"}},
		Resources: &store.ServiceResources{MemoryBytes: 1 << 29, NanoCPUs: 500_000_000},
	}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	if nodeID != "" {
		if err := db.UpdateServiceNode(context.Background(), "web", nodeID); err != nil {
			t.Fatal(err)
		}
	}
	return application.ContainerName("web", application.NameImage(svc), "")
}

func getAppContainers(t *testing.T, rt *Router, db *store.DB) (*httptest.ResponseRecorder, []appContainerResource) {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/containers", ""))
	var got []appContainerResource
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec, got
}

func TestAppContainers_LocalReplica(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	name := seedContainersApp(t, db, "")
	started := time.Now().Add(-time.Hour).UTC()
	fake := &appContainersRuntime{
		fakeExecAppRuntime: &fakeExecAppRuntime{listByPrefixResult: []docker.ContainerState{
			{ID: "0123456789abcdef0123", Name: name, Image: "img:v1", ImageID: "sha256:abc", Running: true,
				Ports:    []docker.PortBinding{{ContainerPort: 8080, HostPort: 33001, HostIP: "127.0.0.1", Protocol: "tcp"}},
				Networks: []docker.ContainerNetwork{{Name: "lr", IPAddress: "172.20.0.5"}}},
			{ID: "other", Name: "web-worker-1234567890ab", Running: true},
		}},
		exit:  &docker.ExitState{Running: true, RestartCount: 2, StartedAt: started},
		stats: docker.ContainerStats{CPUPercent: 12.5, MemoryUsageBytes: 1024, MemoryLimitBytes: 2048},
	}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }

	rec, got := getAppContainers(t, rt, db)
	if rec.Code != http.StatusOK || len(got) != 1 {
		t.Fatalf("status = %d len = %d body = %s", rec.Code, len(got), rec.Body.String())
	}
	c := got[0]
	if c.ID != "0123456789ab" || c.State != "running" || c.Node != "local" || c.ImageDigest != "sha256:abc" {
		t.Errorf("container = %+v", c)
	}
	if c.RestartCount == nil || *c.RestartCount != 2 || c.StartedAt == nil {
		t.Errorf("restart/started = %v %v", c.RestartCount, c.StartedAt)
	}
	if len(c.Ports) != 1 || c.Ports[0].Bind != "127.0.0.1" || c.Ports[0].Host != 33001 {
		t.Errorf("ports = %+v", c.Ports)
	}
	if c.Stats == nil || c.Stats.MemoryBytes != 1024 || c.Limits.CPU != 0.5 || len(c.Mounts) != 1 {
		t.Errorf("stats/limits/mounts = %+v %+v %+v", c.Stats, c.Limits, c.Mounts)
	}
	if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "DB_PASSWORD") {
		t.Fatalf("response leaks env: %s", rec.Body.String())
	}
}

func TestAppContainers_RemoteReplicaMarksUnavailable(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	name := seedContainersApp(t, db, "node-2")
	fake := &fakeExecAppRuntime{listByPrefixResult: []docker.ContainerState{
		{ID: "abc", Name: name, Image: "img:v1", Running: true},
	}}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }

	rec, got := getAppContainers(t, rt, db)
	if rec.Code != http.StatusOK || len(got) != 1 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got[0].Node != "node-2" || got[0].RestartCount != nil || got[0].Stats != nil {
		t.Errorf("container = %+v", got[0])
	}
	have := strings.Join(got[0].Unavailable, ",")
	for _, want := range []string{"health", "restart_count", "started_at", "networks", "stats", "image_digest"} {
		if !strings.Contains(have, want) {
			t.Errorf("unavailable = %q, missing %q", have, want)
		}
	}
}

func TestAppContainers_NoContainersAndNotFound(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	rt.execRuntime = func(string) (docker.Runtime, error) { return &fakeExecAppRuntime{}, nil }
	if rec, _ := getAppContainers(t, rt, db); rec.Code != http.StatusNotFound {
		t.Fatalf("missing app status = %d", rec.Code)
	}
	seedContainersApp(t, db, "")
	rec, got := getAppContainers(t, rt, db)
	if rec.Code != http.StatusOK || len(got) != 0 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAppContainers_ForbiddenCaller(t *testing.T) {
	t.Parallel()
	rt, db, _ := newPipelineRouter(t)
	tok := seedMatrixToken(t, db, "noread", []string{})
	if got := matrixDo(rt, http.MethodGet, "/api/v1/apps/web/containers", bearer(tok)); got != http.StatusForbidden && got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 or 403", got)
	}
	if got := matrixDo(rt, http.MethodGet, "/api/v1/apps/web/containers", nil); got != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", got)
	}
}
