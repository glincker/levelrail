package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fakeMemInfo = "MemTotal:       16384000 kB\n" +
	"MemFree:         2048000 kB\n" +
	"MemAvailable:    8192000 kB\n"

func TestParseMemInfo(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		wantTotal     int64
		wantAvailable int64
		wantErr       bool
	}{
		{
			name:          "typical",
			data:          fakeMemInfo,
			wantTotal:     16384000 * 1024,
			wantAvailable: 8192000 * 1024,
		},
		{
			name:    "missing MemTotal",
			data:    "MemAvailable:    8192000 kB\n",
			wantErr: true,
		},
		{
			name:    "missing MemAvailable",
			data:    "MemTotal:       16384000 kB\n",
			wantErr: true,
		},
		{
			name:    "malformed value",
			data:    "MemTotal:       not-a-number kB\nMemAvailable: 100 kB\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTotal, gotAvailable, err := parseMemInfo([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseMemInfo() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMemInfo() error = %v", err)
			}
			if gotTotal != tt.wantTotal {
				t.Errorf("total = %d, want %d", gotTotal, tt.wantTotal)
			}
			if gotAvailable != tt.wantAvailable {
				t.Errorf("available = %d, want %d", gotAvailable, tt.wantAvailable)
			}
		})
	}
}

func writeFakeMemInfo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "meminfo")
	if err := os.WriteFile(path, []byte(fakeMemInfo), 0o600); err != nil {
		t.Fatalf("write fake meminfo: %v", err)
	}
	return path
}

func TestHostMemoryBytes_RealFile(t *testing.T) {
	total, available, err := hostMemoryBytes(writeFakeMemInfo(t))
	if err != nil {
		t.Fatalf("hostMemoryBytes() error = %v", err)
	}
	if total != 16384000*1024 {
		t.Errorf("total = %d, want %d", total, 16384000*1024)
	}
	if available != 8192000*1024 {
		t.Errorf("available = %d, want %d", available, 8192000*1024)
	}
}

func TestHostMemoryBytes_MissingPath(t *testing.T) {
	if _, _, err := hostMemoryBytes("/no/such/path/levelrail-test"); err == nil {
		t.Error("hostMemoryBytes() error = nil, want an error for a nonexistent path")
	}
}

func TestHostMemoryCollector_CollectOnce_WritesBothMetrics(t *testing.T) {
	db := newTestDB(t)
	c := &HostMemoryCollector{
		memInfoPath: writeFakeMemInfo(t),
		resourceID:  "node:local",
		store:       db,
		interval:    time.Second,
		logger:      slog.Default(),
	}

	if err := c.CollectOnce(context.Background()); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}

	from, to := time.Now().Add(-time.Minute), time.Now().Add(time.Minute)
	total, err := db.Query(context.Background(), "node:local", MetricMemoryTotalBytes, from, to)
	if err != nil {
		t.Fatalf("Query(%s) error = %v", MetricMemoryTotalBytes, err)
	}
	if len(total) != 1 || total[0].Value != float64(16384000*1024) {
		t.Errorf("%s = %+v, want one sample with value %d", MetricMemoryTotalBytes, total, 16384000*1024)
	}

	available, err := db.Query(context.Background(), "node:local", MetricMemoryAvailableBytes, from, to)
	if err != nil {
		t.Fatalf("Query(%s) error = %v", MetricMemoryAvailableBytes, err)
	}
	if len(available) != 1 || available[0].Value != float64(8192000*1024) {
		t.Errorf("%s = %+v, want one sample with value %d", MetricMemoryAvailableBytes, available, 8192000*1024)
	}
}

func TestHostMemoryCollector_CollectOnce_BadPath_ReturnsError(t *testing.T) {
	db := newTestDB(t)
	c := &HostMemoryCollector{
		memInfoPath: "/no/such/path/levelrail-test",
		resourceID:  "node:local",
		store:       db,
		interval:    time.Second,
		logger:      slog.Default(),
	}

	if err := c.CollectOnce(context.Background()); err == nil {
		t.Error("CollectOnce() error = nil, want an error for a nonexistent path")
	}
}

func TestNewHostMemoryCollector_DefaultsToThePlatformSource(t *testing.T) {
	c := NewHostMemoryCollector("node:local", nil, time.Second, nil)
	if c.memInfoPath != "" {
		t.Errorf("memInfoPath = %q, want empty so CollectOnce uses HostMemoryBytes", c.memInfoPath)
	}
}

func TestHostMemoryBytes_ThisHost(t *testing.T) {
	total, available, err := HostMemoryBytes()
	if errors.Is(err, ErrHostMemoryUnsupported) {
		t.Skip("no memory source on this operating system")
	}
	if err != nil {
		t.Fatalf("HostMemoryBytes() error = %v", err)
	}
	if total <= 0 || available <= 0 || available > total {
		t.Fatalf("total=%d available=%d, want 0 < available <= total", total, available)
	}
}
