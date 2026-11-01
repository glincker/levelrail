package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
)

// binfmtFixture points the detector at a temp dir; handlers lists registered
// names, a nil dir means the directory does not exist (undeterminable).
func binfmtFixture(t *testing.T, exists bool, handlers map[string]string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "binfmt_misc")
	if exists {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		for n, body := range handlers {
			if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	old := binfmtDir
	binfmtDir = dir
	t.Cleanup(func() { binfmtDir = old })
}

func platformRun(t *testing.T, imgArch string) (*fakeTrace, error) {
	t.Helper()
	tr := newFakeTrace()
	rt := newTraceRuntime(0)
	rt.pull = pullScript()
	rt.details.Arch = imgArch
	c := traceController(traceDesired(), rt, tr)
	_, err := c.Reconcile(context.Background())
	return tr, err
}

func TestPlatform_MatchingArchPasses(t *testing.T) {
	binfmtFixture(t, true, nil)
	tr, err := platformRun(t, "amd64")
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if s := tr.must(t, deploytrace.StepImageInspect); s.Status != store.TraceStatusDone {
		t.Errorf("status = %q, want done", s.Status)
	}
}

func TestPlatform_MismatchNoEmulationRefuses(t *testing.T) {
	binfmtFixture(t, true, map[string]string{"qemu-arm": "enabled\n"})
	tr, err := platformRun(t, "arm64")
	if err == nil || !strings.Contains(err.Error(), "PlatformMismatch") || !strings.Contains(err.Error(), "install qemu binfmt") {
		t.Fatalf("Reconcile() error = %v, want an actionable PlatformMismatch", err)
	}
	if s := tr.must(t, deploytrace.StepImageInspect); s.Status != store.TraceStatusFailed || s.Reason != "PlatformMismatch" {
		t.Errorf("image_inspect = %+v, want failed PlatformMismatch", s)
	}
}

func TestPlatform_MismatchWithEmulationWarns(t *testing.T) {
	binfmtFixture(t, true, map[string]string{"qemu-aarch64": "enabled\ninterpreter /usr/bin/qemu-aarch64\n"})
	tr, err := platformRun(t, "arm64")
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	s := tr.must(t, deploytrace.StepImageInspect)
	if s.Status != store.TraceStatusWarning || !strings.Contains(s.Detail, "under emulation") {
		t.Errorf("image_inspect = %+v, want an emulation warning", s)
	}
}

func TestPlatform_DisabledHandlerCountsAsMissing(t *testing.T) {
	binfmtFixture(t, true, map[string]string{"qemu-aarch64": "disabled\n"})
	if _, err := platformRun(t, "arm64"); err == nil {
		t.Fatal("a disabled handler must not count as emulation")
	}
}

func TestPlatform_UndeterminableWarns(t *testing.T) {
	binfmtFixture(t, false, nil)
	tr, err := platformRun(t, "arm64")
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	s := tr.must(t, deploytrace.StepImageInspect)
	if s.Status != store.TraceStatusWarning || !strings.Contains(s.Detail, "could not be checked") {
		t.Errorf("image_inspect = %+v, want an unchecked warning", s)
	}
}

func TestPlatform_EnvModes(t *testing.T) {
	tests := []struct {
		mode      string
		handlers  map[string]string
		wantError bool
	}{
		{"warn", nil, false},
		{"refuse", map[string]string{"qemu-aarch64": "enabled\n"}, true},
		{"refuse-when-no-emulation", nil, true},
		{"bogus", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			binfmtFixture(t, true, tt.handlers)
			t.Setenv(EnvPlatformMismatch, tt.mode)
			_, err := platformRun(t, "arm64")
			if (err != nil) != tt.wantError {
				t.Errorf("error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
