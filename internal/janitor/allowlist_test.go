package janitor

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type imageDocker struct {
	fakeDocker
	removed []string
}

func (d *imageDocker) ListAllImages(context.Context) ([]docker.ImageRecord, error) {
	return []docker.ImageRecord{
		{ID: "sha256:aaaaaaaaaaaaaaaa", SizeBytes: 10},
		{ID: "sha256:cccccccccccccccc", SizeBytes: 30},
		{ID: "sha256:bbbbbbbbbbbbbbbb", Tags: []string{"web:1"}, SizeBytes: 20},
	}, nil
}

func (d *imageDocker) RemoveImage(_ context.Context, id string) error {
	d.removed = append(d.removed, id)
	return nil
}

func (d *imageDocker) PruneDanglingImages(context.Context) (docker.PruneImagesResult, error) {
	d.imagesPruned++
	return docker.PruneImagesResult{}, nil
}

func allowEngine(t *testing.T, fs *fakeStore, d Docker, vols []docker.NamedVolume, now *time.Time, removed *[]string) *Engine {
	t.Helper()
	e := New(Deps{
		Store: fs, Docker: d,
		DiskFree:          func(context.Context) (int64, int64, error) { return 80, 100, nil },
		DesiredContainers: func(context.Context) ([]string, error) { return nil, nil },
		OrphanVolumes:     func(context.Context) ([]docker.NamedVolume, error) { return vols, nil },
		RemoveVolume: func(_ context.Context, name string) error {
			*removed = append(*removed, name)
			return nil
		},
		Now: func() time.Time { return *now },
	})
	p := DefaultPolicy()
	p.DryRun = false
	p.DeleteOrphanVolumes = true
	if err := SavePolicy(context.Background(), fs, p, "t", *now); err != nil {
		t.Fatal(err)
	}
	return e
}

func itemNames(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Name)
	}
	slices.Sort(out)
	return out
}

func TestAllowlistActsOnlyOnListedItems(t *testing.T) {
	fs := &fakeStore{
		quarantine: map[string]store.JanitorQuarantine{},
		attempts:   []store.PrunableAttempt{{ID: "a1", ServiceName: "web"}, {ID: "a2", ServiceName: "api"}},
	}
	d := &imageDocker{}
	now := time.Now()
	var removed []string
	vols := []docker.NamedVolume{{Name: "scratch-a", SizeBytes: 1}, {Name: "scratch-b", SizeBytes: 2}, {Name: "db-main", SizeBytes: 3}}
	e := allowEngine(t, fs, d, vols, &now, &removed)

	rep, err := e.Run(context.Background(), Options{Trigger: TriggerManual, Force: true, Items: []ItemSelection{
		{Stage: StageVolumes, Names: []string{"scratch-a", "db-main", "ghost"}},
		{Stage: StageImages, Names: []string{"cccccccccccc"}},
		{Stage: StageAttempts, Names: []string{"api"}},
		{Stage: "bogus", Names: []string{"x"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fs.quarantine["scratch-a"]; !ok || len(fs.quarantine) != 1 {
		t.Errorf("only scratch-a may be quarantined, got %v", fs.quarantine)
	}
	if !slices.Equal(d.removed, []string{"sha256:cccccccccccccccc"}) || d.imagesPruned != 0 {
		t.Errorf("images removed = %v, want only the listed one", d.removed)
	}
	if !slices.Equal(fs.deleted, []string{"a2"}) {
		t.Errorf("attempts deleted = %v, want only api's", fs.deleted)
	}
	if d.containersPruned != 0 || d.cachePruned != 0 {
		t.Error("stages outside the allowlist ran")
	}
	if len(rep.Stages) != 3 {
		t.Errorf("stages = %d, want 3", len(rep.Stages))
	}
	reasons := map[string]string{}
	for _, ig := range rep.Ignored {
		reasons[ig.Name] = ig.Reason
	}
	for _, n := range []string{"db-main", "ghost", "x"} {
		if reasons[n] == "" {
			t.Errorf("%s should be reported as ignored, got %v", n, rep.Ignored)
		}
	}
}

func TestAllowlistDryRunMatchesApply(t *testing.T) {
	sel := []ItemSelection{
		{Stage: StageVolumes, Names: []string{"scratch-a"}},
		{Stage: StageImages, Names: []string{"aaaaaaaaaaaa", "cccccccccccc"}},
		{Stage: StageAttempts, Names: []string{"web"}},
	}
	build := func() (*Engine, *fakeStore) {
		fs := &fakeStore{
			quarantine: map[string]store.JanitorQuarantine{},
			attempts:   []store.PrunableAttempt{{ID: "a1", ServiceName: "web"}, {ID: "a2", ServiceName: "api"}},
		}
		now := time.Now()
		var removed []string
		vols := []docker.NamedVolume{{Name: "scratch-a", SizeBytes: 1}, {Name: "scratch-b", SizeBytes: 2}}
		return allowEngine(t, fs, &imageDocker{}, vols, &now, &removed), fs
	}
	dryEng, dryStore := build()
	dry, err := dryEng.Run(context.Background(), Options{Trigger: TriggerManual, Force: true, DryRun: true, Items: sel})
	if err != nil {
		t.Fatal(err)
	}
	if len(dryStore.quarantine) != 0 || len(dryStore.deleted) != 0 {
		t.Fatal("dry run changed state")
	}
	applyEng, _ := build()
	apply, err := applyEng.Run(context.Background(), Options{Trigger: TriggerManual, Force: true, Items: sel})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []Stage{StageVolumes, StageImages, StageAttempts} {
		got, want := itemNames(stage(t, dry, st).Items), itemNames(stage(t, apply, st).Items)
		if !slices.Equal(got, want) || len(got) == 0 {
			t.Errorf("%s: dry %v vs apply %v", st, got, want)
		}
	}
}

func TestAllowlistNeverRemovesDatabaseVolumes(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{"db-main": {Kind: "volume", Name: "db-main", FirstSeen: time.Now().Add(-90 * 24 * time.Hour)}}}
	now := time.Now()
	var removed []string
	e := allowEngine(t, fs, &fakeDocker{}, []docker.NamedVolume{{Name: "db-main", SizeBytes: 5}}, &now, &removed)
	rep, err := e.Run(context.Background(), Options{Trigger: TriggerManual, Items: []ItemSelection{{Stage: StageVolumes, Names: []string{"db-main"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed %v, a db- volume must never be removed", removed)
	}
	if len(rep.Stages) != 0 || len(rep.Ignored) != 1 {
		t.Errorf("stages=%d ignored=%v, want the request refused and reported", len(rep.Stages), rep.Ignored)
	}
}

func TestRunRecordsActor(t *testing.T) {
	cases := []struct {
		name     string
		opts     Options
		wantKind string
		wantName string
	}{
		{"user", Options{Trigger: TriggerManual, Actor: &Actor{Name: "Ada", Kind: ActorUser}}, ActorUser, "Ada"},
		{"api token", Options{Trigger: TriggerManual, Actor: &Actor{Name: "ci", Kind: ActorToken}}, ActorToken, "ci"},
		{"schedule", Options{Trigger: TriggerScheduled}, ActorSchedule, "scheduler"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}, attempts: []store.PrunableAttempt{{ID: "a1", ServiceName: "web"}}}
			now := time.Now()
			var removed []string
			e := allowEngine(t, fs, &fakeDocker{}, nil, &now, &removed)
			if _, err := e.Run(context.Background(), tc.opts); err != nil {
				t.Fatal(err)
			}
			if len(fs.runs) != 1 || fs.runs[0].ActorKind != tc.wantKind || fs.runs[0].ActorName != tc.wantName {
				t.Fatalf("stored runs = %+v", fs.runs)
			}
			hist, err := e.History(context.Background(), 5)
			if err != nil || len(hist) != 1 || hist[0].Actor == nil || hist[0].Actor.Kind != tc.wantKind {
				t.Fatalf("history = %+v err=%v", hist, err)
			}
		})
	}
}

func TestPolicyChangeAppliesOnNextPass(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}, attempts: []store.PrunableAttempt{{ID: "a1", ServiceName: "web"}}}
	now := time.Now()
	var removed []string
	e := allowEngine(t, fs, &fakeDocker{}, nil, &now, &removed)
	ctx := context.Background()

	first, err := e.Run(ctx, Options{Trigger: TriggerManual, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if stage(t, first, StageAttempts).Status != StatusRan {
		t.Fatal("attempts stage should run before the policy change")
	}
	p, err := LoadPolicy(ctx, fs)
	if err != nil {
		t.Fatal(err)
	}
	p.DisabledStages = []Stage{StageAttempts}
	if err := SavePolicy(ctx, fs, p, "t", now); err != nil {
		t.Fatal(err)
	}
	second, err := e.Run(ctx, Options{Trigger: TriggerManual, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if stage(t, second, StageAttempts).Status != StatusSkipped {
		t.Error("the saved policy must apply on the very next pass without a restart")
	}
}
