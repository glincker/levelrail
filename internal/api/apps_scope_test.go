package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func dashScopeGet[T any](t *testing.T, rt *Router, cookie *http.Cookie, path string) T {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, body = %s", path, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func TestDashboardEndpointsFollowProjectScope(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "p1", Name: "proj", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	for _, n := range []string{"in-proj", "other"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "img:1", Port: 3000}); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}
	if err := db.UpdateServiceProject(ctx, "in-proj", "p1"); err != nil {
		t.Fatalf("assign project: %v", err)
	}
	now := time.Now().UTC()
	if err := tdb.WriteSamples(ctx, []telemetry.Sample{
		{ResourceID: "service:in-proj", Metric: "cpu_percent", Timestamp: now, Value: 5},
		{ResourceID: "service:other", Metric: "cpu_percent", Timestamp: now, Value: 9},
	}); err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}
	for _, n := range []string{"in-proj", "other"} {
		a := store.DeployAttempt{ID: "dep_" + n, ServiceName: n, Image: "img:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusFailed, StartedAt: now.Add(-time.Hour), Error: "boom"}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
		if err := db.FinishDeployAttempt(ctx, a.ID, store.DeployAttemptStatusFailed, now, "boom"); err != nil {
			t.Fatalf("finish attempt: %v", err)
		}
	}
	for _, n := range []string{"db-in", "db-out"} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: n, Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatalf("seed db: %v", err)
		}
	}
	if err := db.UpdateDatabaseProject(ctx, "db-in", "p1"); err != nil {
		t.Fatalf("assign db project: %v", err)
	}

	t.Run("apps-metrics", func(t *testing.T) {
		all := dashScopeGet[[]appMetricsSummary](t, rt, cookie, "/api/v1/apps-metrics")
		got := dashScopeGet[[]appMetricsSummary](t, rt, cookie, "/api/v1/apps-metrics?project=p1")
		if len(all) != 2 || len(got) != 1 || got[0].Name != "in-proj" {
			t.Fatalf("all = %d rows, scoped = %+v", len(all), got)
		}
	})
	t.Run("resource-usage", func(t *testing.T) {
		got := dashScopeGet[[]appResourceUsageResource](t, rt, cookie, "/api/v1/apps/resource-usage?project=p1")
		if len(got) != 1 || got[0].Name != "in-proj" {
			t.Fatalf("scoped = %+v", got)
		}
	})
	t.Run("failed deploys", func(t *testing.T) {
		all := dashScopeGet[[]failedDeployResource](t, rt, cookie, "/api/v1/deploys/failed")
		got := dashScopeGet[[]failedDeployResource](t, rt, cookie, "/api/v1/deploys/failed?project=p1")
		if len(all) != 2 || len(got) != 1 || got[0].ServiceName != "in-proj" {
			t.Fatalf("all = %d rows, scoped = %+v", len(all), got)
		}
	})
	t.Run("sleep fleet", func(t *testing.T) {
		for _, n := range []string{"in-proj", "other"} {
			if err := db.SetAppSleepIdle(ctx, n, 30, now); err != nil {
				t.Fatalf("enable sleep %s: %v", n, err)
			}
		}
		all := dashScopeGet[sleepFleetResource](t, rt, cookie, "/api/v1/sleep")
		got := dashScopeGet[sleepFleetResource](t, rt, cookie, "/api/v1/sleep?project=p1")
		if all.Enabled != 2 || got.Enabled != 1 || got.Apps[0].Name != "in-proj" {
			t.Fatalf("all = %d enabled, scoped = %+v", all.Enabled, got.Apps)
		}
	})
	t.Run("usage summary databases", func(t *testing.T) {
		all := dashScopeGet[usageSummaryResponse](t, rt, cookie, "/api/v1/usage/summary")
		got := dashScopeGet[usageSummaryResponse](t, rt, cookie, "/api/v1/usage/summary?project=p1")
		if len(all.Databases) != 2 || len(got.Databases) != 1 || got.Databases[0].Name != "db-in" {
			t.Fatalf("all = %d dbs, scoped = %+v", len(all.Databases), got.Databases)
		}
	})
}

func TestAdvisorSummaryFollowsProjectScope(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "p1", Name: "proj", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.UpdateDatabaseProject(ctx, "glinr", "p1"); err != nil {
		t.Fatalf("assign db project: %v", err)
	}
	all := dashScopeGet[advisorSummaryResponse](t, rt, cookie, "/api/v1/resource-advisor/summary")
	one := dashScopeGet[advisorSummaryResponse](t, rt, cookie, "/api/v1/resource-advisor/summary?project=p1")
	none := dashScopeGet[advisorSummaryResponse](t, rt, cookie, "/api/v1/resource-advisor/summary?project=empty")
	if one.Actionable == 0 || one.Actionable >= all.Actionable || none.Actionable != 0 {
		t.Fatalf("actionable all=%d project=%d empty=%d", all.Actionable, one.Actionable, none.Actionable)
	}
}
