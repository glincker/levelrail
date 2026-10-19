package fixit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type backupHarness struct {
	eng     *Engine
	stale   bool
	created int
	failErr error
	// noEffect makes Create succeed without freshening the snapshot.
	noEffect bool
}

func newBackupHarness(t *testing.T, withCreate bool) *backupHarness {
	t.Helper()
	h := &backupHarness{stale: true}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h.eng = NewEngine(&memStore{}, WithClock(func() time.Time { return clock }), WithMaxPerHour(3))
	d := BackupDeps{Check: func(context.Context) (CheckState, error) {
		return CheckState{Failing: h.stale, Detail: "newest snapshot is 5 days old"}, nil
	}}
	if withCreate {
		d.Create = func(context.Context) (string, error) {
			if h.failErr != nil {
				return "", h.failErr
			}
			h.created++
			if !h.noEffect {
				h.stale = false
			}
			return "snap-1", nil
		}
	}
	if err := h.eng.Register(ControlPlaneBackupFix(d)); err != nil {
		t.Fatal(err)
	}
	return h
}

const backupFixID = "backup.control_plane.snapshot"

func TestBackupFixPreviewApplyAndIdempotent(t *testing.T) {
	ctx := context.Background()
	h := newBackupHarness(t, true)
	v, err := h.eng.Preview(ctx, backupFixID)
	if err != nil || v.Kind != KindAuto || v.Risk != RiskSafe || len(v.Changes) != 1 || h.created != 0 {
		t.Fatalf("preview must plan without acting: %+v err=%v created=%d", v, err, h.created)
	}
	r, err := h.eng.Apply(ctx, backupFixID, "root", nil)
	if err != nil || r.Outcome != OutcomeFixed || h.created != 1 {
		t.Fatalf("apply: %+v err=%v created=%d", r, err, h.created)
	}
	r, _ = h.eng.Apply(ctx, backupFixID, "root", nil)
	if r.Outcome != OutcomeSkipped || h.created != 1 {
		t.Fatalf("second apply must be a no-op: %+v created=%d", r, h.created)
	}
}

func TestBackupFixHalfSucceededAndLoopProtection(t *testing.T) {
	ctx := context.Background()
	h := newBackupHarness(t, true)
	h.noEffect = true
	r, _ := h.eng.Apply(ctx, backupFixID, "root", nil)
	if r.Outcome != OutcomeStillFailing || r.View.Kind != KindGuided && r.View.Kind != KindManual {
		t.Fatalf("snapshot written but check still failing must demote: %+v", r)
	}
	before := h.created
	_, _ = h.eng.Apply(ctx, backupFixID, "root", nil)
	if h.created != before {
		t.Fatalf("a demoted fix must not run again, created=%d", h.created)
	}
}

func TestBackupFixErrorsAndUnconfigured(t *testing.T) {
	ctx := context.Background()
	h := newBackupHarness(t, true)
	h.failErr = errors.New("disk full")
	r, _ := h.eng.Apply(ctx, backupFixID, "root", nil)
	if r.Outcome != OutcomeStillFailing || h.created != 0 {
		t.Fatalf("create error must surface: %+v", r)
	}
	h2 := newBackupHarness(t, false)
	r, _ = h2.eng.Apply(ctx, backupFixID, "root", nil)
	if r.Outcome != OutcomeCannotFixHere {
		t.Fatalf("no backup manager means cannot fix here: %+v", r)
	}
}

func TestGuidedCheckFixRecheckOnly(t *testing.T) {
	ctx := context.Background()
	failing := true
	eng := NewEngine(&memStore{})
	f := GuidedCheckFix(GuidedDeps{
		ID: "g", CheckID: "firewall", Subject: "host", Title: "t", Why: "w", Risk: RiskSafe,
		Check: func(context.Context) (Finding, error) {
			return Finding{Failing: failing, Detail: "ports closed", Command: "sudo ufw allow 443"}, nil
		},
	})
	if err := eng.Register(f); err != nil {
		t.Fatal(err)
	}
	v, _ := eng.Preview(ctx, "g")
	if v.Kind != KindGuided || v.Command != "sudo ufw allow 443" {
		t.Fatalf("guided view: %+v", v)
	}
	r, _ := eng.Apply(ctx, "g", "root", nil)
	if r.Outcome != OutcomeStillFailing {
		t.Fatalf("recheck while failing: %+v", r)
	}
	failing = false
	r, _ = eng.Apply(ctx, "g", "root", nil)
	if r.Outcome != OutcomeSkipped {
		t.Fatalf("recheck once fixed elsewhere: %+v", r)
	}
	noCmd := GuidedCheckFix(GuidedDeps{
		ID: "n", CheckID: "clock_skew", Subject: "host",
		Check: func(context.Context) (Finding, error) {
			return Finding{Failing: true, Detail: "skew", Reason: "enable NTP"}, nil
		},
	})
	if err := eng.Register(noCmd); err != nil {
		t.Fatal(err)
	}
	v, _ = eng.Preview(ctx, "n")
	if v.Kind != KindManual || v.Reason != "enable NTP" {
		t.Fatalf("no command must be manual with the reason: %+v", v)
	}
}
