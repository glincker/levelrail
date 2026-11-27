package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/imagelife"
)

func TestPlanDiskGC(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	old := now.Add(-72 * time.Hour)
	fresh := now.Add(-time.Hour)
	entries := []imagelife.Entry{
		{ID: "live", SizeBytes: 100, Protected: true, ProtectReason: "live"},
		{ID: "rollback", SizeBytes: 100, Protected: true, ProtectReason: "rollback target"},
		{ID: "pending", SizeBytes: 100, Protected: true, ProtectReason: "pending deploy"},
		{ID: "old", SizeBytes: 50},
		{ID: "shared", SizeBytes: 70, Apps: []imagelife.AppUse{{App: "a"}, {App: "b"}}},
	}
	cache := []docker.BuildCacheRecord{
		{ID: "busy", SizeBytes: 10, InUse: true, LastUsedAt: old},
		{ID: "young", SizeBytes: 20, LastUsedAt: fresh},
		{ID: "stale", SizeBytes: 30, LastUsedAt: old},
	}
	images, _, recs, cacheBytes := planDiskGC(entries, cache, 24*time.Hour, now)
	var got []string
	for _, e := range images {
		got = append(got, e.ID)
	}
	if len(got) != 1 || got[0] != "old" {
		t.Fatalf("removable images = %v, want only old", got)
	}
	if len(recs) != 1 || recs[0].ID != "stale" || cacheBytes != 30 {
		t.Fatalf("removable cache = %v (%d bytes), want only stale", recs, cacheBytes)
	}
}
