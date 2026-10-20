package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TestUsageReport_RealContainerSamples_Live prices the CPU and memory the
// real telemetry collector read from a running container, end to end
// through the report route.
func TestUsageReport_RealContainerSamples_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	const app = "usage-live-app"
	name := "levelrail-usage-live-test"
	if state, err := client.InspectByName(ctx, name); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	} else if state != nil {
		_ = client.Remove(ctx, state.ID, true)
	}
	id, err := client.Create(ctx, docker.ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Skipf("could not create the test container: %v", err)
	}
	t.Cleanup(func() { _ = client.Remove(context.Background(), id, true) })
	if err := client.Start(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}

	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)))
	admin := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "nginx:alpine", Port: 80}); err != nil {
		t.Fatal(err)
	}

	collector := telemetry.NewCollector(telemetry.LocalStatsSource{StatsInspector: client}, tdb, time.Second, discardLogger())
	targets := []telemetry.Target{{ResourceID: "service:" + app, ContainerID: id}}
	for i := 0; i < 4; i++ {
		if err := collector.CollectOnce(ctx, targets); err != nil {
			t.Fatalf("collect: %v", err)
		}
		time.Sleep(time.Second)
	}

	f := &usageFixture{rt: rt, db: db, admin: admin}
	body := jsonBody(t, map[string]any{"settings": map[string]any{
		"mode": "unit", "currency": "USD", "basis": "avg", "cpu_weight_pct": 50, "mem_gib_hour": 100, "cpu_core_hour": 100,
	}})
	if rec := f.do(t, admin, http.MethodPut, "/api/v1/usage/settings", body); rec.Code != http.StatusOK {
		t.Fatalf("put settings = %d: %s", rec.Code, rec.Body.String())
	}
	rep := getUsageReport(t, f, admin, "?period=mtd")
	var found bool
	for _, g := range rep.Resources {
		if g.Name == app {
			found = true
			if g.Total <= 0 || g.Compute <= 0 {
				t.Errorf("real container was not costed: %+v", g)
			}
		}
	}
	if !found {
		t.Fatalf("app missing from report: %+v", rep.Resources)
	}
}
