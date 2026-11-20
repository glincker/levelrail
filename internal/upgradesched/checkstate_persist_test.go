package upgradesched

import (
	"errors"
	"testing"
	"time"
)

func TestCheckFailureStreakSurvivesRestart(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(autoPolicy("any"))
	h.host.latestErr = errors.New("api blocked")
	first := h.sched()
	h.tick(first)
	h.now = h.now.Add(time.Minute)
	h.tick(first)
	if st := first.CheckStatus(); st.Failures != 2 {
		t.Fatalf("failures before restart = %d, want 2", st.Failures)
	}

	lookups := 0
	h.host.onLatest = func() { lookups++ }
	second := h.sched()
	h.tick(second) // still inside the saved backoff
	if lookups != 0 {
		t.Fatalf("a new scheduler looked up inside the saved backoff: %d lookups", lookups)
	}
	st := second.CheckStatus()
	if st.Failures != 2 || st.LastOK || st.LastError != "api blocked" || st.NextCheckAt.IsZero() {
		t.Fatalf("state after restart = %+v", st)
	}
	h.now = st.NextCheckAt.Add(time.Second)
	h.tick(second)
	if got := second.CheckStatus().Failures; got != 3 {
		t.Fatalf("streak did not continue after restart: %d", got)
	}
}

func TestLastGoodListSurvivesFailuresAndRestart(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(autoPolicy("patch"))
	h.host.latest = Candidate{Tag: "v0.6.0", FromFallback: true}
	good := h.now
	s := h.sched()
	h.tick(s)
	if st := s.CheckStatus(); !st.LastGoodAt.Equal(good) || st.Source != SourceFallback {
		t.Fatalf("good lookup = %+v", st)
	}
	h.host.latestErr = ErrStaleList
	h.now = h.now.Add(3 * time.Hour)
	h.tick(s)
	st := h.sched().CheckStatus()
	if !st.LastGoodAt.Equal(good) || st.Failures != 1 || st.LastOK {
		t.Fatalf("stale lookup after restart = %+v, want last good %s kept and one failure", st, good)
	}
}

func TestStaleListStillWeighsTheCandidate(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(autoPolicy("any"))
	h.host.latestErr = ErrStaleList
	h.now = h.now.Add(4 * 24 * time.Hour)
	h.host.latest.PublishedAt = h.now.Add(-10 * 24 * time.Hour)
	s := h.sched()
	h.tick(s)
	if h.active() == nil {
		t.Fatal("an eligible release from the last good list was not scheduled")
	}
	if st := s.CheckStatus(); st.Failures != 1 {
		t.Fatalf("stale list must count as a failed check, got %+v", st)
	}
}
