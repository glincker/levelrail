package api

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/store"
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
	got := rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, kinds, now, false)
	sig, ok := got["web"]
	if !ok || sig.restarts == nil || *sig.restarts != 5 || sig.increasing {
		t.Fatalf("signals = %+v", got)
	}
	rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, kinds, now.Add(time.Second), false)
	if fake.calls != 1 {
		t.Fatalf("inspect calls = %d, want 1 (cache hit)", fake.calls)
	}
}

func TestLiveSummaryMarksIncreasingAfterExpiry(t *testing.T) {
	t.Parallel()
	rt, fake, svc := summaryFixture(t, 1, []int{1})
	kinds := map[string]string{"web": "web"}
	ctx := context.Background()
	rt.liveSummarySignals(ctx, []store.DesiredService{svc}, kinds, time.Now(), false)
	fake.details[0].RestartCount = 4
	rt.appSummaryCache.mu.Lock()
	e := rt.appSummaryCache.m["web"]
	e.at = time.Now().Add(-time.Hour)
	rt.appSummaryCache.m["web"] = e
	rt.appSummaryCache.mu.Unlock()
	got := rt.liveSummarySignals(ctx, []store.DesiredService{svc}, kinds, time.Now(), false)
	if sig := got["web"]; sig.restarts == nil || *sig.restarts != 4 || !sig.increasing {
		t.Fatalf("signals = %+v", got)
	}
}

func TestLiveSummaryOmitsOnFailureAndSkipsCron(t *testing.T) {
	t.Parallel()
	rt, fake, svc := summaryFixture(t, 1, []int{1})
	fake.err = errors.New("node down")
	got := rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, map[string]string{"web": "web"}, time.Now(), false)
	if _, ok := got["web"]; ok {
		t.Fatalf("failure should omit, got %+v", got)
	}
	fake.err = nil
	calls := fake.calls
	got = rt.liveSummarySignals(context.Background(), []store.DesiredService{svc}, map[string]string{"web": appKindCron}, time.Now(), false)
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

func TestLiveSummarySkipsDBSizeExecWithoutAbility(t *testing.T) {
	t.Parallel()
	rt, fake, _ := summaryFixture(t, 1, []int{0})
	ctx := context.Background()
	if err := rt.databases.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	fake.inspectState = &docker.ContainerState{ID: "dbc", Running: true}
	counting := &sizeExecRuntime{inspectingRuntime: fake}
	rt.execRuntime = func(string) (docker.Runtime, error) { return counting, nil }
	svc := store.DesiredService{Name: "main", Image: "pg:16", Replicas: 1}
	kinds := map[string]string{"main": appKindDatabase}
	rt.liveSummarySignals(ctx, []store.DesiredService{svc}, kinds, time.Now(), false)
	if counting.execs != 0 {
		t.Fatalf("exec calls = %d, a low-privilege list must not run the size query", fake.execCalls)
	}
	rt.liveSummarySignals(ctx, []store.DesiredService{svc}, kinds, time.Now(), true)
	if counting.execs == 0 {
		t.Fatal("a caller with the ability should run the size query")
	}
	if _, ok := rt.appSummaryCache.fresh("main", time.Now(), time.Hour); ok {
		if sig, _ := rt.appSummaryCache.fresh("main", time.Now(), time.Hour); sig.dbSize != nil {
			t.Fatal("low tier entry must never hold a size")
		}
	}
}

// sizeExecRuntime counts every exec so a test can prove the size query did or did not run.
type sizeExecRuntime struct {
	*inspectingRuntime
	execs int
}

func (f *sizeExecRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	f.execs++
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *sizeExecRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	f.execs++
	return io.NopCloser(strings.NewReader("")), nil
}
