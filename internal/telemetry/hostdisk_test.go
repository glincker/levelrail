package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestDiskSpaceFromStatfs(t *testing.T) {
	tests := []struct {
		name      string
		blocks    uint64
		bavail    uint64
		bsize     int64
		wantTotal int64
		wantUsed  int64
	}{
		{name: "half used", blocks: 1000, bavail: 500, bsize: 1024, wantTotal: 1024000, wantUsed: 512000},
		{name: "fully available", blocks: 1000, bavail: 1000, bsize: 1024, wantTotal: 1024000, wantUsed: 0},
		{name: "nothing available", blocks: 1000, bavail: 0, bsize: 1024, wantTotal: 1024000, wantUsed: 1024000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTotal, gotUsed := diskSpaceFromStatfs(tt.blocks, tt.bavail, tt.bsize)
			if gotTotal != tt.wantTotal {
				t.Errorf("total = %d, want %d", gotTotal, tt.wantTotal)
			}
			if gotUsed != tt.wantUsed {
				t.Errorf("used = %d, want %d", gotUsed, tt.wantUsed)
			}
		})
	}
}

func TestDiskSpaceBytes_RealPath(t *testing.T) {
	total, used, err := diskSpaceBytes(t.TempDir())
	if err != nil {
		t.Fatalf("diskSpaceBytes() error = %v", err)
	}
	if total <= 0 {
		t.Errorf("total = %d, want > 0", total)
	}
	if used < 0 || used > total {
		t.Errorf("used = %d, want within [0, %d]", used, total)
	}
}

func TestDiskSpaceBytes_MissingPath(t *testing.T) {
	if _, _, err := diskSpaceBytes("/no/such/path/levelrail-test"); err == nil {
		t.Error("diskSpaceBytes() error = nil, want an error for a nonexistent path")
	}
}

func TestHostDiskCollector_CollectOnce_WritesBothMetrics(t *testing.T) {
	db := newTestDB(t)
	c := NewHostDiskCollector(t.TempDir(), "node:local", db, time.Second, nil)

	if err := c.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}

	from, to := time.Now().Add(-time.Minute), time.Now().Add(time.Minute)
	total, err := db.Query(context.Background(), "node:local", "disk_total_bytes", from, to)
	if err != nil {
		t.Fatalf("Query(disk_total_bytes) error = %v", err)
	}
	if len(total) != 1 || total[0].Value <= 0 {
		t.Errorf("disk_total_bytes = %+v, want one sample with a positive value", total)
	}

	used, err := db.Query(context.Background(), "node:local", "disk_used_bytes", from, to)
	if err != nil {
		t.Fatalf("Query(disk_used_bytes) error = %v", err)
	}
	if len(used) != 1 || used[0].Value < 0 {
		t.Errorf("disk_used_bytes = %+v, want one non-negative sample", used)
	}
}

func TestHostDiskCollector_CollectOnce_BadPath_ReturnsError(t *testing.T) {
	db := newTestDB(t)
	c := NewHostDiskCollector("/no/such/path/levelrail-test", "node:local", db, time.Second, nil)

	if err := c.CollectOnce(context.Background()); err == nil {
		t.Error("CollectOnce() error = nil, want an error for a nonexistent path")
	}
}
