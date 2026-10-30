package monitor

import "testing"

func TestTrackerConfirmsAndRecovers(t *testing.T) {
	var got []string
	tr := NewTracker(2, 2, func(x Transition) { got = append(got, x.State) })
	m := Monitor{ID: "mon_1"}
	bad, ok := Result{Reason: "timeout"}, Result{OK: true}

	if s := tr.Observe(m, bad); s != StateSuspect {
		t.Fatalf("first failure: %s", s)
	}
	if s := tr.Observe(m, ok); s != StateUp || len(got) != 0 {
		t.Fatalf("blip must not notify: %s %v", s, got)
	}
	tr.Observe(m, bad)
	if s := tr.Observe(m, bad); s != StateDown || len(got) != 1 || got[0] != StateDown {
		t.Fatalf("confirmed down: %s %v", s, got)
	}
	if s := tr.Observe(m, ok); s != StateRecovering {
		t.Fatalf("first success: %s", s)
	}
	if s := tr.Observe(m, bad); s != StateDown || len(got) != 1 {
		t.Fatalf("relapse must not renotify: %s %v", s, got)
	}
	tr.Observe(m, ok)
	if s := tr.Observe(m, ok); s != StateUp || len(got) != 2 || got[1] != StateUp {
		t.Fatalf("recovered: %s %v", s, got)
	}
}
