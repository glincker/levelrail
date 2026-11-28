package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/attention"
)

func TestDoctorUnitProtectionRowShape(t *testing.T) {
	rt := &Router{dataDir: t.TempDir()}
	got := rt.doctorCheckUnitProtection(context.Background(), time.Now())
	if got.Code != "unit_protection" || got.Status == "" {
		t.Fatalf("unexpected row: %+v", got)
	}
}

func TestUncleanRestartAttentionEmptyWithoutIncidents(t *testing.T) {
	rt := &Router{dataDir: t.TempDir()}
	if items := rt.uncleanRestartAttentionItems(context.Background(), []string{AbilityRead}, time.Now()); len(items) != 0 {
		t.Fatalf("want none, got %+v", items)
	}
	if got := attention.NewItem(attention.Warning, attention.KindUncleanRestart, "control-plane", "x"); got.Title == "" || got.Action == "" {
		t.Fatalf("kind info missing: %+v", got)
	}
}

type protectionFixture struct {
	paths   protectionPaths
	dataDir string
}

func newProtectionFixture(t *testing.T, adj string, dropin bool) protectionFixture {
	t.Helper()
	root := t.TempDir()
	f := protectionFixture{dataDir: filepath.Join(root, "data"), paths: protectionPaths{
		oomScoreAdj: filepath.Join(root, "oom_score_adj"),
		dropinGlob:  filepath.Join(root, "units", "*.service.d", "30-protection.conf"),
		meminfo:     filepath.Join(root, "meminfo"),
	}}
	for path, body := range map[string]string{f.paths.oomScoreAdj: adj + "\n", f.paths.meminfo: "SwapTotal: 0 kB\n"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(f.dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if dropin {
		dir := filepath.Join(root, "units", "levelrail.service.d")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "30-protection.conf"), []byte("# Written by install.sh (levelrail host protection).\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f protectionFixture) addIncident(t *testing.T, at time.Time, oom bool) {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{"at": at, "oom_likely": oom}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dataDir, "unclean-restarts.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestUnitProtectionBranches(t *testing.T) {
	now := time.Now()
	window := 72 * time.Hour
	const limit = -500

	t.Run("protected", func(t *testing.T) {
		f := newProtectionFixture(t, "-900", true)
		got := unitProtectionCheck(f.paths, limit, f.dataDir, window, now)
		if got.Status != doctorStatusOK || !strings.Contains(got.Message, "drop-in present") {
			t.Fatalf("want ok, got %+v", got)
		}
	})
	t.Run("not protected", func(t *testing.T) {
		f := newProtectionFixture(t, "0", false)
		got := unitProtectionCheck(f.paths, limit, f.dataDir, window, now)
		if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "may pick the control plane first") {
			t.Fatalf("want warn, got %+v", got)
		}
	})
	t.Run("drop-in written but not applied", func(t *testing.T) {
		f := newProtectionFixture(t, "0", true)
		got := unitProtectionCheck(f.paths, limit, f.dataDir, window, now)
		if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "not applied yet") {
			t.Fatalf("want warn, got %+v", got)
		}
	})
	t.Run("unclean restart recent", func(t *testing.T) {
		f := newProtectionFixture(t, "-900", true)
		f.addIncident(t, now.Add(-time.Hour), true)
		got := unitProtectionCheck(f.paths, limit, f.dataDir, window, now)
		if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "likely out of memory") {
			t.Fatalf("want warn, got %+v", got)
		}
		if old := unitProtectionCheck(f.paths, limit, f.dataDir, 30*time.Minute, now); old.Status != doctorStatusOK {
			t.Fatalf("an incident outside the window must not warn, got %+v", old)
		}
	})
	t.Run("score unreadable", func(t *testing.T) {
		f := newProtectionFixture(t, "-900", true)
		f.paths.oomScoreAdj = filepath.Join(t.TempDir(), "missing")
		if got := unitProtectionCheck(f.paths, limit, f.dataDir, window, now); got.Status != doctorStatusUnknown {
			t.Fatalf("want unknown, got %+v", got)
		}
	})
}
