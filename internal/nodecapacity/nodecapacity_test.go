package nodecapacity

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEvaluate(t *testing.T) {
	th := ThresholdsFromEnv(func(string) (string, bool) { return "", false })
	cases := []struct {
		name   string
		s      Sample
		worst  Level
		blocks bool
	}{
		{"healthy", Sample{DiskTotal: 100, DiskFree: 50, MemTotal: 100, MemAvail: 50}, LevelOK, false},
		{"disk warn", Sample{DiskTotal: 100, DiskFree: 12}, LevelWarn, false},
		{"disk critical", Sample{DiskTotal: 100, DiskFree: 5}, LevelCritical, true},
		{"inodes critical", Sample{DiskTotal: 100, DiskFree: 90, InodesTotal: 100, InodesFree: 2}, LevelCritical, true},
		{"swap used critical", Sample{SwapTotal: 100, SwapFree: 10}, LevelCritical, true},
		{"unreadable never blocks", Sample{}, LevelOK, false},
	}
	for _, c := range cases {
		st := Evaluate(c.s, th)
		if st.Worst != c.worst || st.Blocking() != c.blocks {
			t.Errorf("%s: worst=%s blocking=%v", c.name, st.Worst, st.Blocking())
		}
	}
}

type fakeStore struct {
	quarantine map[string]store.JanitorQuarantine
}

func (f *fakeStore) GetJanitorPolicy(context.Context) (string, bool, error) { return "", false, nil }
func (f *fakeStore) SetJanitorPolicy(context.Context, string, string, time.Time) error {
	return nil
}
func (f *fakeStore) InsertJanitorRun(context.Context, store.JanitorRun, int) error { return nil }
func (f *fakeStore) ListJanitorRuns(context.Context, int) ([]store.JanitorRun, error) {
	return nil, nil
}
func (f *fakeStore) UpsertJanitorQuarantine(_ context.Context, q store.JanitorQuarantine) error {
	f.quarantine[q.Name] = q
	return nil
}
func (f *fakeStore) ListJanitorQuarantine(context.Context, string) ([]store.JanitorQuarantine, error) {
	return nil, nil
}
func (f *fakeStore) DeleteJanitorQuarantine(context.Context, string, string) error { return nil }
func (f *fakeStore) ListPrunableDeployAttempts(context.Context, time.Time, int, int) ([]store.PrunableAttempt, error) {
	return nil, nil
}
func (f *fakeStore) DeleteDeployAttempts(context.Context, []string) error { return nil }

type fakeDocker struct{ pruned []string }

func (d *fakeDocker) DiskUsage(context.Context) (docker.DiskUsage, error) {
	return docker.DiskUsage{}, nil
}
func (d *fakeDocker) ListAllImages(context.Context) ([]docker.ImageRecord, error) {
	return []docker.ImageRecord{
		{ID: "sha256:dangling000000", SizeBytes: 10},
		{ID: "sha256:rollback000000", Tags: []string{"web:1"}, SizeBytes: 20},
	}, nil
}
func (d *fakeDocker) PruneContainers(context.Context, []string) (docker.PruneContainersResult, error) {
	d.pruned = append(d.pruned, "containers")
	return docker.PruneContainersResult{}, nil
}
func (d *fakeDocker) PruneDanglingImages(context.Context) (docker.PruneImagesResult, error) {
	d.pruned = append(d.pruned, "images")
	return docker.PruneImagesResult{Removed: []string{"sha256:dangling000000"}, ReclaimedBytes: 10}, nil
}
func (d *fakeDocker) PruneAnonymousVolumes(context.Context) (docker.PruneVolumesResult, error) {
	d.pruned = append(d.pruned, "volumes")
	return docker.PruneVolumesResult{}, nil
}
func (d *fakeDocker) BuildCacheRecords(context.Context) ([]docker.BuildCacheRecord, error) {
	return nil, nil
}
func (d *fakeDocker) PruneBuildCacheOlderThan(context.Context, time.Duration) (docker.PruneBuildCacheResult, error) {
	d.pruned = append(d.pruned, "build_cache")
	return docker.PruneBuildCacheResult{}, nil
}

func TestPreviewThenExecuteLeavesRollbackImagesAndVolumes(t *testing.T) {
	fs := &fakeStore{quarantine: map[string]store.JanitorQuarantine{}}
	dk := &fakeDocker{}
	removed := []string{}
	eng := janitor.New(janitor.Deps{
		Store: fs, Docker: dk,
		DiskFree:          func(context.Context) (int64, int64, error) { return 5, 100, nil },
		DesiredContainers: func(context.Context) ([]string, error) { return nil, nil },
		OrphanVolumes: func(context.Context) ([]docker.NamedVolume, error) {
			return []docker.NamedVolume{{Name: "scratch", SizeBytes: 99}}, nil
		},
		RemoveVolume: func(_ context.Context, n string) error { removed = append(removed, n); return nil },
	})

	p, err := BuildPreview(context.Background(), eng, dk)
	if err != nil {
		t.Fatal(err)
	}
	if len(dk.pruned) != 0 || len(fs.quarantine) != 0 {
		t.Fatalf("preview changed state: pruned=%v quarantine=%v", dk.pruned, fs.quarantine)
	}
	byName := map[string]Class{}
	for _, c := range p.Classes {
		byName[c.Name] = c
	}
	if c := byName[ClassDanglingImages]; !c.Deletable || c.Bytes != 10 {
		t.Errorf("dangling class = %+v", c)
	}
	if c := byName[ClassRollbackImages]; c.Deletable || len(c.Items) != 1 || c.Items[0].Name != "web:1" {
		t.Errorf("rollback class = %+v", c)
	}
	if c := byName[ClassOrphanedVolumes]; c.Deletable || len(c.Items) != 1 {
		t.Errorf("volume class = %+v", c)
	}

	res, err := Execute(context.Background(), eng, janitor.TriggerNodeGuardrail)
	if err != nil {
		t.Fatal(err)
	}
	if res.FreedBytes != 10 {
		t.Errorf("freed = %d", res.FreedBytes)
	}
	for _, op := range dk.pruned {
		if op == "volumes" {
			t.Error("volumes were pruned")
		}
	}
	if len(removed) != 0 || len(fs.quarantine) != 0 {
		t.Errorf("volume state touched: removed=%v quarantine=%v", removed, fs.quarantine)
	}
}
