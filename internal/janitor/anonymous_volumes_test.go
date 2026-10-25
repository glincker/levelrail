package janitor

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type anonDocker struct {
	*fakeDocker
	pruned int
}

func (d *anonDocker) ListDanglingAnonymousVolumes(context.Context) ([]docker.AnonymousVolume, error) {
	return []docker.AnonymousVolume{{Name: "aaaa", SizeBytes: 5}, {Name: "bbbb", SizeBytes: 7}}, nil
}

func (d *anonDocker) PruneAnonymousVolumes(context.Context) (docker.PruneVolumesResult, error) {
	d.pruned++
	return docker.PruneVolumesResult{Removed: []string{"aaaa", "bbbb"}, ReclaimedBytes: 12}, nil
}

func runAnon(t *testing.T, mutate func(*Policy), opts Options) (*anonDocker, StageResult, []string) {
	t.Helper()
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}}
	d := &anonDocker{fakeDocker: &fakeDocker{}}
	now := time.Now()
	var removed []string
	e := newEngine(fs, d.fakeDocker, 80, []docker.NamedVolume{{Name: "db-main-data", SizeBytes: 99}}, &now, &removed)
	e.Docker = d
	p := DefaultPolicy()
	p.DryRun = false
	mutate(&p)
	if err := SavePolicy(context.Background(), fs, p, "t", now); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return d, stage(t, rep, StageVolumes), removed
}

func anonItem(t *testing.T, r StageResult) Item {
	t.Helper()
	for _, it := range r.Items {
		if it.Name == "2 anonymous volumes" {
			return it
		}
	}
	t.Fatalf("no anonymous volume item in %+v", r.Items)
	return Item{}
}

func TestAnonymousVolumes_DryRunListsAndRemovesNothing(t *testing.T) {
	d, res, removed := runAnon(t, func(p *Policy) { p.DeleteOrphanVolumes = true }, Options{Trigger: TriggerManual, DryRun: true})
	if d.pruned != 0 || len(removed) != 0 {
		t.Fatalf("dry run removed: pruned=%d removed=%v", d.pruned, removed)
	}
	if it := anonItem(t, res); it.Action != ActionWouldRemove || it.SizeBytes != 12 {
		t.Fatalf("item = %+v", it)
	}
}

func TestAnonymousVolumes_OffByDefaultKeepsThem(t *testing.T) {
	d, res, _ := runAnon(t, func(*Policy) {}, Options{Trigger: TriggerManual})
	if d.pruned != 0 {
		t.Fatal("pruned without the operator enabling volume deletion")
	}
	if it := anonItem(t, res); it.Action != ActionKept {
		t.Fatalf("item = %+v", it)
	}
}

func TestAnonymousVolumes_RemovedOnlyWhenEnabled_NeverNamed(t *testing.T) {
	d, res, removed := runAnon(t, func(p *Policy) { p.DeleteOrphanVolumes = true }, Options{Trigger: TriggerManual})
	if d.pruned != 1 {
		t.Fatalf("pruned = %d, want 1", d.pruned)
	}
	if it := anonItem(t, res); it.Action != ActionRemoved {
		t.Fatalf("item = %+v", it)
	}
	if len(removed) != 0 {
		t.Fatalf("named volumes removed: %v", removed)
	}
}
