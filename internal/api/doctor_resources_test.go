package api

import (
	"errors"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"testing"
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
