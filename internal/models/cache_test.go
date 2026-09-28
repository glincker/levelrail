package models

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCacheBackend struct {
	mu         sync.Mutex
	vols       []docker.NamedVolume
	removed    []string
	failOn     map[string]bool
	mountedNow map[string]bool
}

func (f *fakeCacheBackend) ListVolumesByPrefix(_ context.Context, prefix string) ([]docker.NamedVolume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []docker.NamedVolume
	for _, v := range f.vols {
		if len(v.Name) >= len(prefix) && v.Name[:len(prefix)] == prefix {
			if f.mountedNow[v.Name] {
				v.Mounted = true
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeCacheBackend) RemoveVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn[name] {
		return errors.New("volume is in use")
	}
	f.removed = append(f.removed, name)
	return nil
}

func daysAgo(now time.Time, d int) string {
	return now.Add(-time.Duration(d) * 24 * time.Hour).Format(time.RFC3339Nano)
}

func newCacheSvc(t *testing.T) (*Service, *fakeCacheBackend, *store.DB, time.Time) {
	t.Helper()
	svc, db := newSvc(t, nil)
	be := &fakeCacheBackend{failOn: map[string]bool{}, mountedNow: map[string]bool{}}
	svc.SetCacheBackend(be, "lr")
	now := time.Now().UTC()
	svc.cache.now = func() time.Time { return now }
	return svc, be, db, now
}

func vol(name string, size int64, created string, mounted bool) docker.NamedVolume {
	return docker.NamedVolume{Name: "lr-model-" + name + "-cache", SizeBytes: size, CreatedAt: created, Mounted: mounted}
}

func TestCacheList_ClassifiesEntries(t *testing.T) {
	t.Setenv("APP_MODEL_CACHE_UNUSED_DAYS", "30")
	ctx := context.Background()
	svc, _, _, now := newCacheSvc(t)
	be := svc.cache.backend.(*fakeCacheBackend)
	be.vols = []docker.NamedVolume{
		vol("live", 8*gib, daysAgo(now, 90), true),
		vol("configured", 4*gib, daysAgo(now, 90), false),
		vol("gone-old", 6*gib, daysAgo(now, 45), false),
		vol("gone-new", 2*gib, daysAgo(now, 3), false),
		vol("mounted-orphan", 3*gib, daysAgo(now, 90), true),
		{Name: "lr-model-noise", SizeBytes: 1},
		{Name: "lr-other-volume", SizeBytes: 1},
	}
	for _, name := range []string{"live", "configured"} {
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: name, Engine: EngineOllama, ModelRef: "llama3.1:8b"}}); err != nil {
			t.Fatal(err)
		}
	}

	report, err := svc.CacheList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	local := report.Nodes[0]
	byVol := map[string]CacheEntry{}
	for _, e := range local.Entries {
		byVol[e.Volume] = e
	}
	if len(local.Entries) != 5 {
		t.Fatalf("entries = %d, want 5 (non cache volumes must be skipped): %+v", len(local.Entries), local.Entries)
	}
	want := map[string]bool{"live": false, "configured": false, "gone-old": true, "gone-new": false, "mounted-orphan": false}
	for name, prunable := range want {
		e := byVol["lr-model-"+name+"-cache"]
		if e.Prunable != prunable {
			t.Errorf("%s prunable = %v, want %v (keep: %q)", name, e.Prunable, prunable, e.KeepReason)
		}
		if !prunable && e.KeepReason == "" {
			t.Errorf("%s kept without a reason", name)
		}
	}
	if e := byVol["lr-model-gone-old-cache"]; e.UnusedDays < 44 || !e.Unused || e.LastUsedSource != "volume_created" {
		t.Errorf("gone-old = %+v", e)
	}
	if e := byVol["lr-model-configured-cache"]; e.Model != "configured" || e.LastUsedSource != "model_updated" {
		t.Errorf("configured = %+v", e)
	}
	if local.ReclaimableBytes != 6*gib {
		t.Errorf("reclaimable = %d, want %d", local.ReclaimableBytes, 6*gib)
	}
}

func TestCacheList_DuplicateWeightsCountOnce(t *testing.T) {
	ctx := context.Background()
	svc, be, _, now := newCacheSvc(t)
	be.vols = []docker.NamedVolume{
		vol("chat-a", 8*gib, daysAgo(now, 1), true),
		vol("chat-b", 8*gib, daysAgo(now, 1), true),
		vol("other", 3*gib, daysAgo(now, 1), true),
	}
	for name, ref := range map[string]string{"chat-a": "llama3.1:8b", "chat-b": "Llama3.1:8b", "other": "phi3:mini"} {
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: name, Engine: EngineOllama, ModelRef: ref}}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := svc.CacheList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := report.Nodes[0]
	dups := 0
	for _, e := range n.Entries {
		if e.DuplicateOf != "" {
			dups++
		}
	}
	if n.TotalBytes != 19*gib || n.UniqueBytes != 11*gib || dups != 1 {
		t.Errorf("total=%d unique=%d dups=%d, want %d %d 1", n.TotalBytes, n.UniqueBytes, dups, 19*gib, 11*gib)
	}
}

func TestCacheList_RecentGatewayTrafficKeepsConfiguredModelFresh(t *testing.T) {
	ctx := context.Background()
	svc, be, db, now := newCacheSvc(t)
	be.vols = []docker.NamedVolume{vol("busy", gib, daysAgo(now, 200), false)}
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("busy")}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddModelUsage(ctx, []store.ModelUsage{{ModelName: "busy", KeyID: "default", HourStart: now.Add(-2 * time.Hour).Truncate(time.Hour), Requests: 5}}); err != nil {
		t.Fatal(err)
	}
	report, _ := svc.CacheList(ctx)
	e := report.Nodes[0].Entries[0]
	if e.LastUsedSource != "gateway_traffic" || e.Unused || e.UnusedDays != 0 {
		t.Errorf("entry = %+v", e)
	}
}

func TestCachePrune_DryRunRemovesNothing(t *testing.T) {
	ctx := context.Background()
	svc, be, _, now := newCacheSvc(t)
	be.vols = []docker.NamedVolume{vol("old", 5*gib, daysAgo(now, 90), false)}
	res, err := svc.CachePrune(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || len(res.Candidates) != 1 || len(res.Removed) != 0 || len(be.removed) != 0 {
		t.Errorf("res = %+v removed = %v", res, be.removed)
	}
}

func TestCachePrune_NeverDeletesInUseFiles(t *testing.T) {
	ctx := context.Background()
	svc, be, _, now := newCacheSvc(t)
	be.vols = []docker.NamedVolume{
		vol("running", 9*gib, daysAgo(now, 400), true),
		vol("configured-stopped", 9*gib, daysAgo(now, 400), false),
		vol("orphan", 5*gib, daysAgo(now, 90), false),
	}
	for _, n := range []string{"running", "configured-stopped"} {
		if _, err := svc.Create(ctx, CreateInput{Spec: spec(n)}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("everything mode only takes the orphan", func(t *testing.T) {
		res, err := svc.CachePrune(ctx, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(be.removed) != 1 || be.removed[0] != "lr-model-orphan-cache" || res.ReclaimedBytes != 5*gib {
			t.Errorf("removed %v res %+v", be.removed, res)
		}
	})

	t.Run("naming a protected volume explicitly is refused", func(t *testing.T) {
		be.removed = nil
		res, err := svc.CachePrune(ctx, []string{"lr-model-running-cache", "lr-model-configured-stopped-cache", "lr-model-ghost-cache"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(be.removed) != 0 || len(res.Skipped) != 3 {
			t.Errorf("removed %v skipped %+v", be.removed, res.Skipped)
		}
	})
}

func TestCachePrune_RechecksRightBeforeRemoving(t *testing.T) {
	ctx := context.Background()
	svc, be, _, now := newCacheSvc(t)
	be.vols = []docker.NamedVolume{vol("racy", 5*gib, daysAgo(now, 90), false), vol("mounted-late", 5*gib, daysAgo(now, 90), false)}
	report, err := svc.localCache(ctx, 30)
	if err != nil || len(report.Entries) != 2 {
		t.Fatalf("setup: %v %+v", err, report)
	}
	be.mountedNow["lr-model-mounted-late-cache"] = true

	if reason := svc.stillPrunable(ctx, CacheEntry{Volume: "lr-model-mounted-late-cache"}); reason != "mounted by a container" {
		t.Errorf("late mount not caught: %q", reason)
	}
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("racy")}); err != nil {
		t.Fatal(err)
	}
	if reason := svc.stillPrunable(ctx, CacheEntry{Volume: "lr-model-racy-cache"}); reason == "" {
		t.Error("model created after listing was not caught")
	}
}

func TestCachePrune_RemoveFailureIsReportedNotFatal(t *testing.T) {
	ctx := context.Background()
	svc, be, _, now := newCacheSvc(t)
	be.vols = []docker.NamedVolume{vol("a", gib, daysAgo(now, 90), false), vol("b", 2*gib, daysAgo(now, 90), false)}
	be.failOn["lr-model-a-cache"] = true
	res, err := svc.CachePrune(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "lr-model-b-cache" || len(res.Skipped) != 1 || res.ReclaimedBytes != 2*gib {
		t.Errorf("res = %+v", res)
	}
}

func TestCacheUnusedDays_Env(t *testing.T) {
	t.Setenv("APP_MODEL_CACHE_UNUSED_DAYS", "7")
	if CacheUnusedDays() != 7 {
		t.Error("env ignored")
	}
	t.Setenv("APP_MODEL_CACHE_UNUSED_DAYS", "-2")
	if CacheUnusedDays() != 30 {
		t.Error("bad value must fall back to 30")
	}
}
