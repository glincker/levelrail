package resilience

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

func TestSnapshotRestartDryRun(t *testing.T) {
	snap, err := ingress.NewSnapshotStore(filepath.Join(t.TempDir(), ingress.SnapshotFilename), make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	d := ingress.New(nil)
	d.SetSnapshot(snap, "bound")
	if err := d.SaveSnapshotNow(); !errors.Is(err, ingress.ErrNoAppliedConfig) {
		t.Fatalf("snapshot with nothing applied: %v", err)
	}
	if _, err := d.VerifySnapshot(); !errors.Is(err, ingress.ErrNoSnapshot) {
		t.Fatalf("verify before any save: %v", err)
	}
	if err := snap.Save(ingress.SnapshotData{SavedAt: time.Now(), Fingerprint: "bound", Config: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	chk, err := d.VerifySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	in := IngressInput{SnapshotEnabled: true, SavedAt: chk.SavedAt, Verified: true, ConfigBytes: chk.ConfigBytes, CertKeys: 3}
	rep := Evaluate(time.Now(), true, in, []NodeInput{{ID: "n1", Name: "n1", Status: "online"}})
	if got := rep.Scopes[0].Checks[0]; got.Status != StatusPass {
		t.Fatalf("valid snapshot: %+v", got)
	}

	d2 := ingress.New(nil)
	d2.SetSnapshot(snap, "fd:3:4")
	_, in.VerifyErr = d2.VerifySnapshot()
	rep = Evaluate(time.Now(), true, in, nil)
	if got := rep.Scopes[0].Checks[0]; got.Status != StatusFail || got.Fix != FixSaveSnapshot {
		t.Fatalf("stale snapshot must fail with a fix: %+v", got)
	}
	if rep.Overall != StatusFail {
		t.Fatalf("overall %s", rep.Overall)
	}
}
