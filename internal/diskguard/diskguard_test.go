package diskguard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func fakeStat(free *int64) StatFunc {
	return func(string) (int64, int64, error) { return *free, 100 * 1024 * mib, nil }
}

func testThresholds() Thresholds {
	return Thresholds{LowFree: 100 * mib, CriticalFree: 50 * mib, Reserve: 2 * mib}
}

func TestThresholdsFromEnv(t *testing.T) {
	env := map[string]string{EnvLowFreeMB: "200", EnvCriticalFreeMB: "300", EnvReserveMB: "bad"}
	th := ThresholdsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if th.LowFree != 200*mib || th.CriticalFree != 200*mib || th.Reserve != defaultReserveMB*mib {
		t.Fatalf("thresholds = %+v", th)
	}
}

func TestGuardRefuseByLevelAndOp(t *testing.T) {
	tests := []struct {
		name    string
		free    int64
		op      Op
		refused bool
	}{
		{"plenty", 500 * mib, OpBuild, false},
		{"low still allowed", 80 * mib, OpBuild, false},
		{"critical build", 10 * mib, OpBuild, true},
		{"critical pull", 10 * mib, OpPull, true},
		{"critical backup", 10 * mib, OpBackup, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			free := tc.free
			g := New(t.TempDir(), "/docker", testThresholds(), fakeStat(&free), nil)
			err := g.Refuse(tc.op)
			var low *LowDiskError
			if tc.refused != errors.As(err, &low) {
				t.Fatalf("Refuse(%s) = %v, refused want %v", tc.op, err, tc.refused)
			}
			if tc.refused && !strings.Contains(err.Error(), "Free space now") {
				t.Fatalf("error %q has no next action", err)
			}
		})
	}
}

func TestGuardUnreadableVolumeNeverRefuses(t *testing.T) {
	g := New(t.TempDir(), "", testThresholds(), func(string) (int64, int64, error) { return 0, 0, errors.New("boom") }, nil)
	if err := g.Refuse(OpBuild); err != nil {
		t.Fatalf("Refuse() = %v, want nil for an unreadable volume", err)
	}
	if st := g.Status(); st.Volumes[0].Level != LevelUnknown {
		t.Fatalf("level = %s, want unknown", st.Volumes[0].Level)
	}
}

func TestTickReserveReleasedAtCriticalAndRestored(t *testing.T) {
	dir := t.TempDir()
	free := 500 * mib
	g := New(dir, "", testThresholds(), fakeStat(&free), nil)
	ctx := context.Background()

	_ = g.Tick(ctx)
	fi, err := os.Stat(filepath.Join(dir, reserveFileName))
	if err != nil || fi.Size() != 2*mib {
		t.Fatalf("reserve = %v, %v, want a 2 MiB file", fi, err)
	}

	free = 10 * mib
	_ = g.Tick(ctx)
	if _, err := os.Stat(filepath.Join(dir, reserveFileName)); !os.IsNotExist(err) {
		t.Fatalf("reserve still present at critical: %v", err)
	}

	free = 500 * mib
	_ = g.Tick(ctx)
	if _, err := os.Stat(filepath.Join(dir, reserveFileName)); err != nil {
		t.Fatalf("reserve not restored after recovery: %v", err)
	}
}

func TestTickNotifiesOncePerDrop(t *testing.T) {
	free := 500 * mib
	g := New(t.TempDir(), "", testThresholds(), fakeStat(&free), nil)
	var msgs []string
	g.Notify = func(_ context.Context, s string) { msgs = append(msgs, s) }
	ctx := context.Background()

	_ = g.Tick(ctx)
	free = 80 * mib
	_ = g.Tick(ctx)
	_ = g.Tick(ctx)
	free = 10 * mib
	_ = g.Tick(ctx)
	if len(msgs) != 2 {
		t.Fatalf("notices = %d (%v), want 2 (low, then critical)", len(msgs), msgs)
	}
}

func TestIsNoSpace(t *testing.T) {
	if !IsNoSpace(&os.PathError{Op: "write", Path: "x", Err: syscall.ENOSPC}) || IsNoSpace(errors.New("other")) {
		t.Fatal("IsNoSpace misclassified")
	}
}
