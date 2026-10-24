package janitor

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	policy     string
	runs       []store.JanitorRun
	quarantine map[string]store.JanitorQuarantine
	attempts   []store.PrunableAttempt
	deleted    []string
}

func (f *fakeStore) GetJanitorPolicy(context.Context) (string, bool, error) {
	return f.policy, f.policy != "", nil
}

func (f *fakeStore) SetJanitorPolicy(_ context.Context, body, _ string, _ time.Time) error {
	f.policy = body
	return nil
}

func (f *fakeStore) InsertJanitorRun(_ context.Context, r store.JanitorRun, _ int) error {
	f.runs = append(f.runs, r)
	return nil
}

func (f *fakeStore) ListJanitorRuns(context.Context, int) ([]store.JanitorRun, error) {
	return f.runs, nil
}

func (f *fakeStore) UpsertJanitorQuarantine(_ context.Context, q store.JanitorQuarantine) error {
	if _, ok := f.quarantine[q.Name]; !ok {
		f.quarantine[q.Name] = q
	}
	return nil
}

func (f *fakeStore) ListJanitorQuarantine(context.Context, string) ([]store.JanitorQuarantine, error) {
	var out []store.JanitorQuarantine
	for _, q := range f.quarantine {
		out = append(out, q)
	}
	return out, nil
}

func (f *fakeStore) DeleteJanitorQuarantine(_ context.Context, _, name string) error {
	delete(f.quarantine, name)
	return nil
}

func (f *fakeStore) ListPrunableDeployAttempts(context.Context, time.Time, int, int) ([]store.PrunableAttempt, error) {
	return f.attempts, nil
}

func (f *fakeStore) DeleteDeployAttempts(_ context.Context, ids []string) error {
	f.deleted = append(f.deleted, ids...)
	return nil
}

func (f *fakeStore) PurgeInactiveAPITokens(context.Context, store.TokenPurgeKind, time.Time, int, bool) (int, error) {
	return 0, nil
}

type fakeDocker struct {
	containersPruned, imagesPruned, cachePruned int
}

func (d *fakeDocker) DiskUsage(context.Context) (docker.DiskUsage, error) {
	return docker.DiskUsage{ContainersReclaimableBytes: 100}, nil
}

func (d *fakeDocker) ListAllImages(context.Context) ([]docker.ImageRecord, error) {
	return []docker.ImageRecord{
		{ID: "sha256:aaaaaaaaaaaaaaaa", SizeBytes: 10},
		{ID: "sha256:bbbbbbbbbbbbbbbb", Tags: []string{"web:1"}, SizeBytes: 20},
	}, nil
}

func (d *fakeDocker) PruneContainers(context.Context, []string) (docker.PruneContainersResult, error) {
	d.containersPruned++
	return docker.PruneContainersResult{Removed: []string{"old"}, ReclaimedBytes: 5}, nil
}

func (d *fakeDocker) PruneDanglingImages(context.Context) (docker.PruneImagesResult, error) {
	d.imagesPruned++
	return docker.PruneImagesResult{Removed: []string{"sha256:aaaaaaaaaaaaaaaa"}, ReclaimedBytes: 10}, nil
}

func (d *fakeDocker) PruneAnonymousVolumes(context.Context) (docker.PruneVolumesResult, error) {
	return docker.PruneVolumesResult{}, nil
}

func (d *fakeDocker) BuildCacheRecords(context.Context) ([]docker.BuildCacheRecord, error) {
	old := time.Now().Add(-30 * 24 * time.Hour)
	return []docker.BuildCacheRecord{
		{ID: "a", SizeBytes: 50, LastUsedAt: old},
		{ID: "b", SizeBytes: 70, InUse: true, LastUsedAt: old},
	}, nil
}

func (d *fakeDocker) PruneBuildCacheOlderThan(context.Context, time.Duration) (docker.PruneBuildCacheResult, error) {
	d.cachePruned++
	return docker.PruneBuildCacheResult{Removed: []string{"a"}, ReclaimedBytes: 50}, nil
}

func newEngine(fs *fakeStore, d *fakeDocker, freePct int64, vols []docker.NamedVolume, now *time.Time, removed *[]string) *Engine {
	return New(Deps{
		Store: fs, Docker: d,
		DiskFree:          func(context.Context) (int64, int64, error) { return freePct, 100, nil },
		DesiredContainers: func(context.Context) ([]string, error) { return []string{"web"}, nil },
		OrphanVolumes:     func(context.Context) ([]docker.NamedVolume, error) { return vols, nil },
		RemoveVolume: func(_ context.Context, name string) error {
			*removed = append(*removed, name)
			return nil
		},
		Now: func() time.Time { return *now },
	})
}

func stage(t *testing.T, rep Report, s Stage) StageResult {
	t.Helper()
	for _, r := range rep.Stages {
		if r.Stage == s {
			return r
		}
	}
	t.Fatalf("stage %s missing", s)
	return StageResult{}
}

func TestDryRunRemovesNothing(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}, attempts: []store.PrunableAttempt{{ID: "x", ServiceName: "web"}}}
	d := &fakeDocker{}
	now := time.Now()
	var removed []string
	e := newEngine(fs, d, 5, []docker.NamedVolume{{Name: "scratch", SizeBytes: 9}}, &now, &removed)

	rep, err := e.Run(context.Background(), Options{Trigger: TriggerManual, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Pressure {
		t.Error("5 percent free is below the 15 percent budget")
	}
	if d.containersPruned+d.imagesPruned+d.cachePruned != 0 || len(removed) != 0 || len(fs.deleted) != 0 || len(fs.quarantine) != 0 {
		t.Fatal("dry run changed state")
	}
	if got := stage(t, rep, StageImages).Items; len(got) != 1 || got[0].Action != ActionWouldRemove {
		t.Errorf("images preview = %+v, want only the dangling image", got)
	}
	if got := stage(t, rep, StageVolumes).Items; len(got) != 1 || got[0].Action != ActionWouldQuarantine {
		t.Errorf("volumes preview = %+v", got)
	}
}

func TestNoPressureSkipsSizeDrivenStages(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}}
	d := &fakeDocker{}
	now := time.Now()
	var removed []string
	e := newEngine(fs, d, 80, nil, &now, &removed)
	rep, err := e.Run(context.Background(), Options{Trigger: TriggerManual})
	if err != nil {
		t.Fatal(err)
	}
	if d.containersPruned != 0 || d.imagesPruned != 0 {
		t.Error("containers and images must wait for disk pressure")
	}
	if stage(t, rep, StageContainers).Status != StatusSkipped {
		t.Error("containers stage should be skipped")
	}
	if _, err := e.Run(context.Background(), Options{Trigger: TriggerManual, Force: true}); err != nil {
		t.Fatal(err)
	}
	if d.containersPruned != 1 || d.imagesPruned != 1 {
		t.Error("force must run the size-driven stages")
	}
}

func TestVolumeQuarantineThenDelete(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}}
	d := &fakeDocker{}
	now := time.Now()
	var removed []string
	vols := []docker.NamedVolume{
		{Name: "scratch", SizeBytes: 9},
		{Name: "db-main-data", SizeBytes: 99},
		{Name: "mounted", Mounted: true},
	}
	e := newEngine(fs, d, 80, vols, &now, &removed)
	ctx := context.Background()
	if err := SavePolicy(ctx, fs, func() Policy { p := DefaultPolicy(); p.DryRun = false; p.DeleteOrphanVolumes = true; return p }(), "t", now); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Run(ctx, Options{Trigger: TriggerManual}); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 || len(fs.quarantine) != 1 {
		t.Fatalf("first pass must only quarantine scratch: removed=%v quarantine=%v", removed, fs.quarantine)
	}

	now = now.Add(5 * 24 * time.Hour)
	if _, err := e.Run(ctx, Options{Trigger: TriggerManual}); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatal("volume deleted inside its quarantine")
	}

	now = now.Add(20 * 24 * time.Hour)
	rep, err := e.Run(ctx, Options{Trigger: TriggerManual})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "scratch" {
		t.Fatalf("removed = %v, want only scratch (never db-* or mounted)", removed)
	}
	if got := stage(t, rep, StageVolumes).ReclaimedBytes; got != 9 {
		t.Errorf("reclaimed = %d, want 9", got)
	}
}

func TestVolumeLeavesQuarantineWhenReclaimed(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{"back": {Kind: "volume", Name: "back", FirstSeen: time.Now()}}}
	now := time.Now()
	var removed []string
	e := newEngine(fs, &fakeDocker{}, 80, nil, &now, &removed)
	if _, err := e.Run(context.Background(), Options{Trigger: TriggerManual}); err != nil {
		t.Fatal(err)
	}
	if len(fs.quarantine) != 0 {
		t.Fatal("a volume that is referenced again must leave quarantine")
	}
}

func TestAttemptsDeleted(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}, attempts: []store.PrunableAttempt{{ID: "a1", ServiceName: "web"}, {ID: "a2", ServiceName: "web"}}}
	now := time.Now()
	var removed []string
	e := newEngine(fs, &fakeDocker{}, 80, nil, &now, &removed)
	rep, err := e.Run(context.Background(), Options{Trigger: TriggerManual})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs.deleted) != 2 {
		t.Fatalf("deleted = %v", fs.deleted)
	}
	if items := stage(t, rep, StageAttempts).Items; len(items) != 1 || items[0].Action != ActionRemoved {
		t.Errorf("items = %+v", items)
	}
	if len(fs.runs) != 1 {
		t.Errorf("manual runs are recorded, got %d", len(fs.runs))
	}
}

func TestPolicyValidate(t *testing.T) {
	cases := map[string]func(*Policy){
		"interval":   func(p *Policy) { p.IntervalMinutes = 1 },
		"budget":     func(p *Policy) { p.DiskFreeBudgetPercent = 0 },
		"quarantine": func(p *Policy) { p.VolumeQuarantineDays = 0 },
		"keep":       func(p *Policy) { p.AttemptKeepPerApp = 0 },
		"stage":      func(p *Policy) { p.DisabledStages = []Stage{"nope"} },
	}
	for name, mutate := range cases {
		p := DefaultPolicy()
		mutate(&p)
		if p.Validate() == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	if err := DefaultPolicy().Validate(); err != nil {
		t.Errorf("default policy invalid: %v", err)
	}
}

func TestDisabledStageIsSkipped(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}, attempts: []store.PrunableAttempt{{ID: "a1", ServiceName: "web"}}}
	now := time.Now()
	var removed []string
	e := newEngine(fs, &fakeDocker{}, 80, nil, &now, &removed)
	p := DefaultPolicy()
	p.DisabledStages = []Stage{StageAttempts}
	if err := SavePolicy(context.Background(), fs, p, "t", now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), Options{Trigger: TriggerManual}); err != nil {
		t.Fatal(err)
	}
	if len(fs.deleted) != 0 {
		t.Fatal("disabled stage ran")
	}
}
