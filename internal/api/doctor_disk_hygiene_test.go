package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestDiskHygieneSuggestsLargestReclaimable(t *testing.T) {
	c := diskConsumers(docker.DiskUsage{
		ImagesTotalBytes: 5 << 30, ImagesReclaimableBytes: 1 << 30,
		BuildCacheTotalBytes: 3 << 30, BuildCacheReclaimableBytes: 2 << 30,
	})
	if c[0].name != "images" {
		t.Fatalf("largest first, got %s", c[0].name)
	}
	if got := suggestedCleanup(c); !strings.Contains(got, "docker builder prune") {
		t.Fatalf("want build cache cleanup, got %q", got)
	}
	if got := suggestedCleanup(nil); !strings.Contains(got, "larger disk") {
		t.Fatalf("fallback missing: %q", got)
	}
}

func TestDiskHygieneCheckUnknownWithoutSources(t *testing.T) {
	rt := &Router{}
	if got := rt.doctorCheckDiskHygiene(t.Context()); got.Status != doctorStatusUnknown {
		t.Fatalf("status %q", got.Status)
	}
}
