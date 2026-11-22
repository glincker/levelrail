package agent

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/nodecapacity"
)

type cleanupRuntime struct {
	docker.Runtime
	keptSeen []string
	pruned   bool
}

func (c *cleanupRuntime) DiskUsage(context.Context) (docker.DiskUsage, error) {
	return docker.DiskUsage{}, nil
}

func (c *cleanupRuntime) ListAllImages(context.Context) ([]docker.ImageRecord, error) {
	return []docker.ImageRecord{{ID: "sha256:aaaaaaaaaaaaaaaa", SizeBytes: 10}}, nil
}

func (c *cleanupRuntime) PruneContainers(_ context.Context, keep []string) (docker.PruneContainersResult, error) {
	c.keptSeen, c.pruned = keep, true
	return docker.PruneContainersResult{Removed: []string{"old"}, ReclaimedBytes: 5}, nil
}

func (c *cleanupRuntime) PruneDanglingImages(context.Context) (docker.PruneImagesResult, error) {
	return docker.PruneImagesResult{Removed: []string{"sha256:aaaaaaaaaaaaaaaa"}, ReclaimedBytes: 10}, nil
}

func (c *cleanupRuntime) PruneAnonymousVolumes(context.Context) (docker.PruneVolumesResult, error) {
	return docker.PruneVolumesResult{}, nil
}

func (c *cleanupRuntime) BuildCacheRecords(context.Context) ([]docker.BuildCacheRecord, error) {
	return nil, nil
}

func (c *cleanupRuntime) PruneBuildCacheOlderThan(context.Context, time.Duration) (docker.PruneBuildCacheResult, error) {
	return docker.PruneBuildCacheResult{}, nil
}

func TestExecuteCleanupPreviewThenRun(t *testing.T) {
	rt := &cleanupRuntime{}
	req := &agentpb.CleanupRequest{DryRun: true, KeepContainers: []string{"web"}}
	out, err := executeCleanup(context.Background(), rt, req)
	if err != nil {
		t.Fatal(err)
	}
	if rt.pruned || out.GetFreedBytes() != 0 {
		t.Fatalf("a preview must remove nothing, pruned=%v freed=%d", rt.pruned, out.GetFreedBytes())
	}
	for _, c := range out.GetClasses() {
		if c.GetName() == nodecapacity.ClassOrphanedVolumes {
			t.Fatal("the node preview must not list volumes")
		}
	}

	req.DryRun = false
	out, err = executeCleanup(context.Background(), rt, req)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.pruned || len(rt.keptSeen) != 1 || rt.keptSeen[0] != "web" {
		t.Fatalf("the desired names must reach the prune, got %v", rt.keptSeen)
	}
	if out.GetFreedBytes() != 15 {
		t.Fatalf("freed = %d, want 15", out.GetFreedBytes())
	}
}
