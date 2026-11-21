package nodeinsight

import (
	"testing"
	"time"
)

func TestVersionHealth_RollbackSetsHoldAndKeepsLastGood(t *testing.T) {
	var h VersionHealth
	now := time.Now()
	h.Observe("v1.0.0", now)
	h.Observe("v1.1.0", now)
	h.Observe("v1.0.0", now)
	if h.RolledBackFrom != "v1.1.0" || h.LastGood != "v1.0.0" {
		t.Fatalf("state = %+v", h)
	}
}

func TestDecideUpdate_HoldsAfterRollbackUntilTargetChanges(t *testing.T) {
	f := UpdateFacts{Channel: "stable", ControlPlane: "v1.1.0", AgentVersion: "v1.0.0", AgentOnline: true, RolledBackFrom: "v1.1.0"}
	if d := DecideUpdate(f); d.Action != UpdateHold {
		t.Errorf("action = %q, want hold", d.Action)
	}
	f.ControlPlane = "v1.2.0"
	if d := DecideUpdate(f); d.Action != UpdateInstall {
		t.Errorf("action = %q, want update to the new target", d.Action)
	}
}

func TestVersionTracker_ClearRolledBack(t *testing.T) {
	var tr VersionTracker
	now := time.Now()
	tr.Observe("n", "v1.0.0", now)
	tr.Observe("n", "v1.1.0", now)
	if h := tr.Observe("n", "v1.0.0", now); h.RolledBackFrom == "" {
		t.Fatal("hold not set")
	}
	tr.ClearRolledBack("n")
	if h := tr.Observe("n", "v1.0.0", now); h.RolledBackFrom != "" {
		t.Errorf("hold not cleared: %+v", h)
	}
}

func TestOfflineSince_IgnoresOfflineWithoutHandshake(t *testing.T) {
	since := time.Now()
	ev := func(to string, d time.Duration) Event { return Event{To: to, At: since.Add(d)} }
	if n := OfflineSince([]Event{ev("offline", time.Second)}, since); n != 0 {
		t.Errorf("offline before any online counted: %d", n)
	}
	if n := OfflineSince([]Event{ev("offline", 3*time.Second), ev("online", time.Second)}, since); n != 1 {
		t.Errorf("offline after online = %d, want 1 even with newest-first order", n)
	}
}
