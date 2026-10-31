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

func TestTrackerStatusCounts(t *testing.T) {
	tr := NewTracker(3, 2, nil)
	m := Monitor{ID: "mon_2"}
	if st := tr.Status(m.ID); st.State != "" {
		t.Fatalf("unprobed: %+v", st)
	}
	tr.Observe(m, Result{})
	tr.Observe(m, Result{})
	if st := tr.Status(m.ID); st.State != StateSuspect || st.FailStreak != 2 {
		t.Fatalf("suspect: %+v", st)
	}
}

func TestTrackerKeepsCountingWhileDown(t *testing.T) {
	tr := NewTracker(2, 2, nil)
	m := Monitor{ID: "mon_3"}
	for range 6 {
		tr.Observe(m, Result{Reason: "refused"})
	}
	if st := tr.Status(m.ID); st.State != StateDown || st.FailStreak != 6 {
		t.Fatalf("down streak must keep growing past the global threshold: %+v", st)
	}
}
