package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestDoctorCheckRAM(t *testing.T) {
	tests := []struct {
		name       string
		read       func() (int64, int64, error)
		minBytes   int64
		wantStatus string
	}{
		{
			name:       "well above the minimum",
			read:       func() (int64, int64, error) { return 8 << 30, 4 << 30, nil },
			wantStatus: doctorStatusOK,
		},
		{
			name:       "documented 1 GB VM passes with default floor",
			read:       func() (int64, int64, error) { return 950 << 20, 500 << 20, nil },
			wantStatus: doctorStatusOK,
		},
		{
			name:       "below the configured minimum",
			read:       func() (int64, int64, error) { return 512 << 20, 256 << 20, nil },
			minBytes:   1 << 30,
			wantStatus: doctorStatusWarn,
		},
		{
			name:       "unsupported operating system is a clean unknown",
			read:       func() (int64, int64, error) { return 0, 0, telemetry.ErrHostMemoryUnsupported },
			wantStatus: doctorStatusUnknown,
		},
		{
			name:       "read error degrades to unknown",
			read:       func() (int64, int64, error) { return 0, 0, errors.New("read /proc/meminfo: permission denied") },
			wantStatus: doctorStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newDoctorTestRouter(t)
			rt.doctorHostMemory = tt.read
			rt.doctorMinRAMBytes = tt.minBytes
			c := rt.doctorCheckRAM()
			if c.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message %q)", c.Status, tt.wantStatus, c.Message)
			}
			if tt.wantStatus == doctorStatusWarn && c.Fix == "" {
				t.Error("Fix = \"\", want a concrete suggestion")
			}
		})
	}
}

func TestDoctorCheckCPU(t *testing.T) {
	tests := []struct {
		name       string
		count      func() int
		minCount   int
		wantStatus string
	}{
		{
			name:       "well above the minimum",
			count:      func() int { return 8 },
			wantStatus: doctorStatusOK,
		},
		{
			name:       "documented single vCPU passes with default floor",
			count:      func() int { return 1 },
			wantStatus: doctorStatusOK,
		},
		{
			name:       "below the configured minimum",
			count:      func() int { return 1 },
			minCount:   4,
			wantStatus: doctorStatusWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newDoctorTestRouter(t)
			rt.doctorNumCPU = tt.count
			rt.doctorMinCPUCount = tt.minCount
			c := rt.doctorCheckCPU()
			if c.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message %q)", c.Status, tt.wantStatus, c.Message)
			}
			if tt.wantStatus == doctorStatusWarn && c.Fix == "" {
				t.Error("Fix = \"\", want a concrete suggestion")
			}
		})
	}
}

func TestDoctorCheckBuildMemory(t *testing.T) {
	for _, tt := range []struct {
		name  string
		total int64
		want  string
	}{
		{"1 GB warns", 1 << 30, doctorStatusWarn},
		{"4 GB ok", 4 << 30, doctorStatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newDoctorTestRouter(t)
			rt.doctorHostMemory = func() (int64, int64, error) { return tt.total, tt.total, nil }
			if c := rt.doctorCheckBuildMemory(t.Context()); c.Status != tt.want {
				t.Errorf("status = %q, want %q", c.Status, tt.want)
			}
		})
	}
}

func TestDoctorCheckBuildMemory_StoredSetting(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	rt.doctorHostMemory = func() (int64, int64, error) { return 6 << 30, 6 << 30, nil }
	svc := runtimesettings.New(db, func(string) (string, bool) { return "", false })
	rt.runtimeSettings = svc
	ctx := t.Context()

	if _, err := svc.Set(ctx, runtimesettings.KeyBuildsMemoryLimitMB, json.RawMessage(`512`), "test"); err != nil {
		t.Fatal(err)
	}
	c := rt.doctorCheckBuildMemory(ctx)
	if c.Status != doctorStatusOK || !strings.Contains(c.Message, "capped at 512 MiB") {
		t.Errorf("saved 512: %+v", c)
	}
	if _, err := svc.Set(ctx, runtimesettings.KeyBuildsMemoryLimitMB, json.RawMessage(`99999`), "test"); err != nil {
		t.Fatal(err)
	}
	c = rt.doctorCheckBuildMemory(ctx)
	if c.Status != doctorStatusWarn || !strings.Contains(c.Message, "99999") || c.Fix == "" {
		t.Errorf("saved above host RAM: %+v", c)
	}
}

func TestDoctorCheckBuildMemory_LiveChangeAndReset(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	rt.doctorHostMemory = func() (int64, int64, error) { return 6 << 30, 6 << 30, nil }
	svc := runtimesettings.New(db, func(string) (string, bool) { return "", false })
	rt.runtimeSettings = svc
	ctx := t.Context()

	c := rt.doctorCheckBuildMemory(ctx)
	if !strings.Contains(c.Message, "when the setting is auto") {
		t.Fatalf("unset: %+v", c)
	}
	if _, err := svc.Set(ctx, runtimesettings.KeyBuildsMemoryLimitMB, json.RawMessage(`1024`), "test"); err != nil {
		t.Fatal(err)
	}
	if c = rt.doctorCheckBuildMemory(ctx); !strings.Contains(c.Message, "capped at 1024 MiB by the builds.memory_limit_mb") {
		t.Errorf("saved: %+v", c)
	}
	if _, err := svc.Set(ctx, runtimesettings.KeyBuildsMemoryLimitMB, json.RawMessage(`0`), "test"); err != nil {
		t.Fatal(err)
	}
	if c = rt.doctorCheckBuildMemory(ctx); !strings.Contains(c.Message, "when the setting is auto") {
		t.Errorf("saved 0 is auto again: %+v", c)
	}
}
