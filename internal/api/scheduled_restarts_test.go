package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func restartAuditCount(t *testing.T, db *store.DB) map[string]int {
	t.Helper()
	entries, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	out := map[string]int{}
	for _, e := range entries {
		if strings.HasPrefix(e.Action, "app.restart_schedule.") {
			out[e.Action]++
		}
	}
	return out
}

// seedDueRestart stores a schedule already due, pinned to the app's current life.
func seedDueRestart(t *testing.T, db *store.DB, name string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	inc, err := db.EnsureServiceIncarnation(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	past := now.Add(-time.Minute).Format(time.RFC3339)
	if err := db.SetAppRestartSchedule(ctx, store.AppRestartSchedule{AppName: name, Schedule: "0 3 * * *", Timezone: "UTC", Incarnation: inc, SetAt: past, NextRunAt: past}); err != nil {
		t.Fatal(err)
	}
}

func restartNonce(t *testing.T, db *store.DB) string {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	return svc.RestartNonce
}

func TestRestartSchedule_FloorEnforced(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	for _, expr := range []string{"*/5 * * * *", "@every 30m", "@every 1s", "0,30 * * * *"} {
		if _, err := parseRestartSchedule(expr, time.UTC, now, time.Hour); err == nil {
			t.Errorf("%q passed a 1h floor", expr)
		}
	}
	for _, expr := range []string{"0 3 * * *", "@every 6h", "@daily"} {
		if _, err := parseRestartSchedule(expr, time.UTC, now, time.Hour); err != nil {
			t.Errorf("%q refused: %v", expr, err)
		}
	}
	if _, err := parseRestartSchedule("not a cron", time.UTC, now, time.Hour); err == nil {
		t.Error("invalid cron accepted")
	}
}

func TestRestartSchedule_DueAppRestartedOnceAndAudited(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedApp(t, db, "web")
	now := time.Now().UTC()
	seedDueRestart(t, db, "web", now)
	before := restartNonce(t, db)
	if n, err := rt.SweepAppRestartSchedules(ctx, now); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v; want 1 restart", n, err)
	}
	after := restartNonce(t, db)
	if after == before {
		t.Fatal("restart nonce unchanged")
	}
	if n, _ := rt.SweepAppRestartSchedules(ctx, now); n != 0 {
		t.Fatalf("second sweep restarted %d apps", n)
	}
	s, _ := db.GetAppRestartSchedule(ctx, "web")
	if s == nil || s.LastStatus != store.AppRestartStatusSucceeded || s.NextRunAt <= now.Format(time.RFC3339) {
		t.Fatalf("row after run = %+v", s)
	}
	if got := restartAuditCount(t, db); got["app.restart_schedule.run"] != 1 {
		t.Fatalf("audit = %v", got)
	}
}

func TestRestartSchedule_SkippedNeverForced(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		setup  func(t *testing.T, db *store.DB)
		reason string
	}{
		"stopped": {func(t *testing.T, db *store.DB) {
			if err := db.UpdateServiceSuspended(context.Background(), "app", true); err != nil {
				t.Fatal(err)
			}
		}, "stopped"},
		"protected environment": {func(t *testing.T, db *store.DB) {
			ctx := context.Background()
			if err := db.SaveProject(ctx, store.Project{ID: "p1", Name: "p1", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
				t.Fatal(err)
			}
			if err := db.SaveEnvironment(ctx, store.Environment{ID: "env-prod", ProjectID: "p1", Name: "production", Protected: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
				t.Fatal(err)
			}
			if err := db.SetServiceEnvironment(ctx, "app", "env-prod"); err != nil {
				t.Fatal(err)
			}
		}, "protected environment"},
		"freeze": {func(t *testing.T, db *store.DB) {
			w := store.DeployFreezeWindow{Scope: store.DeployFreezeScopeApp("app"), Cron: "* * * * *", Duration: 2 * time.Hour, Timezone: "UTC", Reason: "launch"}
			if _, err := db.ReplaceDeployFreezeWindows(context.Background(), w.Scope, []store.DeployFreezeWindow{w}); err != nil {
				t.Fatal(err)
			}
		}, "freeze"},
		"running deploy": {func(t *testing.T, db *store.DB) {
			if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{ID: "att1", ServiceName: "app", Image: "img:v1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}, "deploy is running"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rt, db := newTestRouter(t)
			rt.deploySafety = db
			ctx := context.Background()
			seedApp(t, db, "app")
			tc.setup(t, db)
			now := time.Now().UTC()
			seedDueRestart(t, db, "app", now)
			before, _ := db.GetDesiredService(ctx, "app")
			if n, err := rt.SweepAppRestartSchedules(ctx, now); err != nil || n != 0 {
				t.Fatalf("sweep = %d, %v; want a skip", n, err)
			}
			after, _ := db.GetDesiredService(ctx, "app")
			if after.RestartNonce != before.RestartNonce || after.Suspended != before.Suspended {
				t.Fatal("skipped app was restarted or started")
			}
			s, _ := db.GetAppRestartSchedule(ctx, "app")
			if s.LastStatus != store.AppRestartStatusSkipped || !strings.Contains(s.LastReason, tc.reason) {
				t.Fatalf("row = %+v, want skipped with %q", s, tc.reason)
			}
			if s.NextRunAt <= now.Format(time.RFC3339) {
				t.Fatal("skip did not move the next run forward")
			}
			if got := restartAuditCount(t, db); got["app.restart_schedule.skipped"] != 1 {
				t.Fatalf("audit = %v", got)
			}
		})
	}
}

func TestRestartSchedule_RecreatedAppDoesNotInherit(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedApp(t, db, "web")
	now := time.Now().UTC()
	seedDueRestart(t, db, "web", now)
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	seedApp(t, db, "web")
	if s, _ := db.GetAppRestartSchedule(ctx, "web"); s != nil {
		t.Fatalf("recreated app inherited %+v", s)
	}
	// A stale row from another life is dropped by the sweep, never fired.
	if _, err := db.EnsureServiceIncarnation(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	past := now.Add(-time.Minute).Format(time.RFC3339)
	if err := db.SetAppRestartSchedule(ctx, store.AppRestartSchedule{AppName: "web", Schedule: "0 3 * * *", Timezone: "UTC", Incarnation: "old-life", SetAt: past, NextRunAt: past}); err != nil {
		t.Fatal(err)
	}
	before := restartNonce(t, db)
	if n, err := rt.SweepAppRestartSchedules(ctx, now); err != nil || n != 0 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	if restartNonce(t, db) != before {
		t.Fatal("new app restarted by an old schedule")
	}
	if s, _ := db.GetAppRestartSchedule(ctx, "web"); s != nil {
		t.Fatal("stale row not removed")
	}
}

// backoffTestNow is mid-day UTC so the retry is never capped by the 03:00 cron
// the seeded schedule fires at, whatever time the suite runs.
func backoffTestNow() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 12, 0, 0, 0, time.UTC)
}

type failingRestartStore struct{ *store.DB }

func (failingRestartStore) RestartService(context.Context, string) error {
	return errors.New("restart unavailable")
}

func TestRestartSchedule_FailureBacksOff(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	rt.apps = failingRestartStore{db}
	ctx := context.Background()
	seedApp(t, db, "web")
	now := backoffTestNow()
	seedDueRestart(t, db, "web", now)
	if _, err := rt.SweepAppRestartSchedules(ctx, now); err == nil {
		t.Fatal("failed restart reported no error")
	}
	s, _ := db.GetAppRestartSchedule(ctx, "web")
	if s.LastStatus != store.AppRestartStatusFailed || s.Failures != 1 {
		t.Fatalf("row = %+v", s)
	}
	next, _ := time.Parse(time.RFC3339, s.NextRunAt)
	if next.Sub(now) < restartBackoffBounds.def {
		t.Fatalf("retry after %s, want at least the backoff", next.Sub(now))
	}
	// Sweeping again inside the backoff does nothing: no tight retry loop.
	if _, err := rt.SweepAppRestartSchedules(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if s2, _ := db.GetAppRestartSchedule(ctx, "web"); s2.Failures != 1 {
		t.Fatalf("retried inside backoff: %+v", s2)
	}
	if restartBackoff(time.Minute, 3) != 4*time.Minute || restartBackoff(time.Hour, 40) != restartBackoffCeiling {
		t.Fatal("backoff does not double and cap")
	}
}

func TestRestartSchedule_WriteTokenCannotSet(t *testing.T) {
	t.Parallel()
	assertPlainWriteTokenForbidden(t, http.MethodPut, "/api/v1/apps/web/restart-schedule", `{"schedule":"@every 24h"}`,
		"tok_write", "write-scoped-token", //nolint:gosec // fake fixture, not a real credential
		"a scheduled restart applies staged env like a manual restart, so it needs deploy")
	assertPlainWriteTokenForbidden(t, http.MethodDelete, "/api/v1/apps/web/restart-schedule", "",
		"tok_write", "write-scoped-token", //nolint:gosec // fake fixture, not a real credential
		"clearing a schedule needs deploy too")
}

func TestRestartSchedule_RePutKeepsFloorAfterRun(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedApp(t, db, "web")
	now := time.Now().UTC().Truncate(time.Second)
	seedDueRestart(t, db, "web", now)
	if n, err := rt.SweepAppRestartSchedules(ctx, now); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	rec := doJSON(t, rt, http.MethodPut, "/api/v1/apps/web/restart-schedule", `{"schedule":"@every 1h"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-put = %d %s", rec.Code, rec.Body.String())
	}
	s, _ := db.GetAppRestartSchedule(ctx, "web")
	next, _ := time.Parse(time.RFC3339, s.NextRunAt)
	if next.Before(now.Add(restartMinIntervalBounds.def)) {
		t.Fatalf("re-put moved next run to %s, inside the floor after %s", s.NextRunAt, now)
	}
	if n, _ := rt.SweepAppRestartSchedules(ctx, now.Add(time.Minute)); n != 0 {
		t.Fatal("re-put let the app restart again within the floor")
	}
}

type failingPolicyStore struct{ *store.DB }

func (failingPolicyStore) GetRolloutPolicy(context.Context, string) (store.RolloutPolicy, bool, error) {
	return store.RolloutPolicy{}, false, errors.New("policy unavailable")
}

func TestRestartSchedule_PolicyReadErrorBacksOffNotRestart(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	rt.apps = failingPolicyStore{db}
	ctx := context.Background()
	seedApp(t, db, "web")
	now := backoffTestNow()
	seedDueRestart(t, db, "web", now)
	before := restartNonce(t, db)
	if _, err := rt.SweepAppRestartSchedules(ctx, now); err == nil {
		t.Fatal("unreadable budget reported no error")
	}
	if restartNonce(t, db) != before {
		t.Fatal("restarted with an unreadable disruption budget")
	}
	s, _ := db.GetAppRestartSchedule(ctx, "web")
	next, _ := time.Parse(time.RFC3339, s.NextRunAt)
	if s.Failures != 1 || s.LastStatus != store.AppRestartStatusFailed || next.Sub(now) < restartBackoffBounds.def {
		t.Fatalf("row = %+v, want a backed off failure", s)
	}
}

type erroringLookupStore struct{ *store.DB }

func (s erroringLookupStore) GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error) {
	if strings.HasPrefix(name, "bad") {
		return nil, errors.New("lookup unavailable")
	}
	return s.DB.GetDesiredService(ctx, name)
}

func TestRestartSchedule_ErroringRowsDoNotStarveHealthy(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	rt.apps = erroringLookupStore{db}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for _, n := range []string{"bad1", "bad2", "bad3", "bad4"} {
		seedApp(t, db, n)
		seedDueRestart(t, db, n, now.Add(-time.Hour))
	}
	seedApp(t, db, "web")
	seedDueRestart(t, db, "web", now)
	restarted := 0
	for range 3 {
		n, _ := rt.sweepAppRestartSchedules(ctx, now, 3)
		restarted += n
	}
	if restarted != 1 {
		t.Fatalf("healthy app restarted %d times, want 1", restarted)
	}
	for _, n := range []string{"bad1", "bad2", "bad3", "bad4"} {
		if s, _ := db.GetAppRestartSchedule(ctx, n); s.Failures != 1 {
			t.Fatalf("%s row = %+v, want one backed off failure", n, s)
		}
	}
}
