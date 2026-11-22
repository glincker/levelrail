package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestQuota_DefaultsFollowRuntimeSettingsLive(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	ctx := context.Background()
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyQuotaDefaultMaxApps, json.RawMessage("1"), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyQuotaDefaultMaxCPU, json.RawMessage(`"1.5"`), "test"); err != nil {
		t.Fatal(err)
	}
	if got := rt.quotaDefaults(ctx); got.Apps != 1 || got.NanoCPUs != 1_500_000_000 {
		t.Fatalf("defaults = %+v, want 1 app and 1.5 CPU", got)
	}
	if code, body := createQuotaApp(t, rt, cookie, "a1", proj, ""); code != http.StatusCreated {
		t.Fatalf("first create = %d %s", code, body)
	}
	if code, _ := createQuotaApp(t, rt, cookie, "a2", proj, ""); code != http.StatusConflict {
		t.Fatalf("second create = %d, want 409 from the live default", code)
	}
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyQuotaDefaultMaxApps, json.RawMessage("0"), "test"); err != nil {
		t.Fatal(err)
	}
	if code, body := createQuotaApp(t, rt, cookie, "a2", proj, ""); code != http.StatusCreated {
		t.Fatalf("create after lifting the default = %d %s, want 201 with no restart", code, body)
	}
}

func TestQuota_LiveUsageSumsMetricsOfScopeApps(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_live", Name: "live", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"in1", "in2"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "img:1", Port: 3000}); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceProject(ctx, n, "proj_live"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "other", Image: "img:1", Port: 3001}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := tdb.WriteSamples(ctx, []telemetry.Sample{
		{ResourceID: "service:in1", Metric: "cpu_percent", Timestamp: now, Value: 50},
		{ResourceID: "service:in1", Metric: "memory_usage_bytes", Timestamp: now, Value: 256 << 20},
		{ResourceID: "service:in2", Metric: "cpu_percent", Timestamp: now, Value: 100},
		{ResourceID: "service:other", Metric: "cpu_percent", Timestamp: now, Value: 400},
		{ResourceID: "service:other", Metric: "memory_usage_bytes", Timestamp: now, Value: 1 << 30},
	}); err != nil {
		t.Fatal(err)
	}
	code, body := quotaDo(t, rt, cookie, http.MethodGet, "/api/v1/projects/proj_live/quota", "")
	if code != http.StatusOK {
		t.Fatalf("get = %d %s", code, body)
	}
	var res quotaResource
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if res.Live == nil || res.Live.MemoryMB != 256 || res.Live.CPU != 1.5 || res.Live.SampledApps != 2 || res.Live.TotalApps != 2 {
		t.Fatalf("live = %+v, want 256 MB, 1.5 CPU over 2 of 2 apps (other project excluded)", res.Live)
	}
}
