package selfupgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestApplyBackupFailureNeedsSkipBackup(t *testing.T) {
	boom := errors.New("disk full")

	h := newFakeHost(t)
	h.backupErr = boom
	a, err := Apply(context.Background(), h.deps(), req())
	if a.Outcome != OutcomeFailed || !errors.Is(err, boom) || !strings.Contains(err.Error(), "--skip-backup") {
		t.Fatalf("outcome=%s err=%v, want a refusal that names --skip-backup", a.Outcome, err)
	}
	if h.read(h.bin) != "OLD" || h.stops != 0 {
		t.Fatalf("host changed despite the failed backup: binary=%s stops=%d", h.read(h.bin), h.stops)
	}

	h = newFakeHost(t)
	h.backupErr = boom
	d := h.deps()
	d.SkipBackup = true
	a, err = Apply(context.Background(), d, req())
	if err != nil || a.Outcome != OutcomeSucceeded || h.read(h.bin) != "NEW" {
		t.Fatalf("skip-backup outcome=%s err=%v binary=%s", a.Outcome, err, h.read(h.bin))
	}
	if a.SnapshotPath != "" {
		t.Fatalf("snapshot path %q recorded for a skipped backup", a.SnapshotPath)
	}
}

func TestPlanDisk(t *testing.T) {
	const mib = 1 << 20
	if r := PlanDisk(100*mib, nil, 10*mib, 20*mib, ""); !r.OK || r.RequiredBytes != 70*mib {
		t.Fatalf("roomy disk: %+v", r)
	}
	if r := PlanDisk(50*mib, nil, 10*mib, 20*mib, ""); r.OK {
		t.Fatalf("short disk reported ok: %+v", r)
	}
	if r := PlanDisk(100*mib, nil, mib, mib, "209715200"); r.OK || r.RequiredBytes != 200*mib {
		t.Fatalf("env floor ignored: %+v", r)
	}
	if r := PlanDisk(0, errors.New("x"), mib, mib, ""); r.Known || r.OK {
		t.Fatalf("unreadable disk must be unknown: %+v", r)
	}
}
