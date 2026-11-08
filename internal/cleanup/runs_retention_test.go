package cleanup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

type trimRuns struct {
	*fakeRuns
	keepSeen int
}

func (f *trimRuns) TrimCleanupRuns(_ context.Context, _ string, keep int) (int64, error) {
	f.keepSeen = keep
	return 0, nil
}

func TestCleanupRunsTrimUsesSetting(t *testing.T) {
	h := newHarness(t, tgt("a.b", OriginExisting, 0))
	tr := &trimRuns{fakeRuns: h.runs}
	h.c.cfg.Store = tr
	h.set.ints[runtimesettings.KeyCleanupRunsKeep] = 42
	h.c.trimRuns(t.Context())
	if tr.keepSeen != 42 {
		t.Fatalf("keep = %d, want 42", tr.keepSeen)
	}
}

func TestCleanupRunsPauseAfterConsecutiveFailures(t *testing.T) {
	tg := tgt("a.b", OriginExisting, 0)
	tg.err = errors.New("boom")
	h := newHarness(t, tg)
	h.set.ints["retention.a.b"] = 90
	h.set.ints[runtimesettings.KeyCleanupMaxFailures] = 2
	run := func() { _, _, _ = h.c.Run(t.Context(), "a.b", Request{Opts: RunOptions{Trigger: TriggerCLI}}) }
	run()
	if h.c.paused(t.Context(), "a.b") {
		t.Fatal("one failure must not pause")
	}
	run()
	if !h.c.paused(t.Context(), "a.b") {
		t.Fatal("two failures must pause")
	}
	pauses := 0
	for _, n := range h.notes {
		if strings.Contains(n, "paused") {
			pauses++
		}
	}
	if pauses != 1 {
		t.Fatalf("pause alerts = %d, want 1: %v", pauses, h.notes)
	}
	run()
	for _, n := range h.notes {
		if strings.Count(n, "paused") > 1 {
			t.Fatal("duplicate pause alert")
		}
	}
	if len(h.notes) != 2 {
		t.Fatalf("a third failure must not alert again: %v", h.notes)
	}
	tg.err = nil
	run()
	if h.c.paused(t.Context(), "a.b") {
		t.Fatal("a successful run must resume the target")
	}
}
