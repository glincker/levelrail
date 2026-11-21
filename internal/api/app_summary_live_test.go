package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func summaryFixture(t *testing.T, replicas int, restarts []int) (*Router, *inspectingRuntime, store.DesiredService) {
	t.Helper()
	rt, db := newTestRouter(t)
	svc := store.DesiredService{Name: "web", Image: "img:v1", Port: 8080, Replicas: replicas}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	fake := &inspectingRuntime{fakeExecAppRuntime: &fakeExecAppRuntime{}}
	for i, n := range restarts {
		fake.details = append(fake.details, docker.ContainerDetail{
			ID: "c" + string(rune('a'+i)), Name: application.ReplicaContainerName("web", "img:v1", "", i), RestartCount: n,
		})
	}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }
	return rt, fake, svc
}

func TestLiveSummarySumsRestartsAndCaches(t *testing.T) {
	t.Parallel()
	rt, fake, svc := summaryFixture(t, 2, []int{2, 3})
	kinds := map[string]string{"web": "web"}
	now := time.Now()
	got := rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, kinds, now)
	sig, ok := got["web"]
	if !ok || sig.restarts == nil || *sig.restarts != 5 || sig.increasing {
		t.Fatalf("signals = %+v", got)
	}
	rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, kinds, now.Add(time.Second))
	if fake.calls != 1 {
		t.Fatalf("inspect calls = %d, want 1 (cache hit)", fake.calls)
	}
}

func TestLiveSummaryMarksIncreasingAfterExpiry(t *testing.T) {
	t.Parallel()
	rt, fake, svc := summaryFixture(t, 1, []int{1})
	kinds := map[string]string{"web": "web"}
	ctx := context.Background()
	rt.liveSummarySignals(ctx, []store.DesiredService{svc}, kinds, time.Now())
	fake.details[0].RestartCount = 4
	rt.appSummaryCache.mu.Lock()
	e := rt.appSummaryCache.m["web"]
	e.at = time.Now().Add(-time.Hour)
	rt.appSummaryCache.m["web"] = e
	rt.appSummaryCache.mu.Unlock()
	got := rt.liveSummarySignals(ctx, []store.DesiredService{svc}, kinds, time.Now())
	if sig := got["web"]; sig.restarts == nil || *sig.restarts != 4 || !sig.increasing {
		t.Fatalf("signals = %+v", got)
	}
}

func TestLiveSummaryOmitsOnFailureAndSkipsCron(t *testing.T) {
	t.Parallel()
	rt, fake, svc := summaryFixture(t, 1, []int{1})
	fake.err = errors.New("node down")
	got := rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, map[string]string{"web": "web"}, time.Now())
	if _, ok := got["web"]; ok {
		t.Fatalf("failure should omit, got %+v", got)
	}
	fake.err = nil
	calls := fake.calls
	got = rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, map[string]string{"web": appKindCron}, time.Now())
	if len(got) != 0 || fake.calls != calls {
		t.Fatalf("cron app must not be inspected: %+v", got)
	}
}

func TestSummaryCacheEvictsOldest(t *testing.T) {
	t.Parallel()
	var c summaryCache
	base := time.Now()
	n := 1
	for i, name := range []string{"a", "b", "c"} {
		c.put(name, liveSignals{restarts: &n}, base.Add(time.Duration(i)*time.Second), 2)
	}
	if _, ok := c.m["a"]; ok || len(c.m) != 2 {
		t.Fatalf("cache = %+v", c.m)
	}
}
