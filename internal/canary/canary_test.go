package canary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func sample(reqs, errs uint64, bucket int) Sample {
	s := Sample{Requests: reqs, Errors: errs}
	s.Latency[bucket] = reqs
	return s
}

func testPolicy() Policy {
	return Policy{Stages: []int{10, 50, 100}, Interval: time.Minute, MaxErrorRate: 0.02, MaxP95Ratio: 1.5}
}

func testLimits() Limits { return Limits{MinRequests: 30, P95FloorMS: 50, Tick: time.Second} }

func TestDecide(t *testing.T) {
	const fast, slow = 2, 7 // 25ms and 1000ms buckets
	tests := []struct {
		name string
		in   Input
		want Action
	}{
		{"early and healthy holds", Input{Elapsed: 10 * time.Second, Canary: sample(100, 0, fast), Baseline: sample(900, 0, fast)}, Hold},
		{"interval over but too little traffic holds", Input{Elapsed: 2 * time.Minute, Canary: sample(5, 0, fast), Baseline: sample(900, 0, fast)}, Hold},
		{"healthy advances", Input{Elapsed: time.Minute, Canary: sample(100, 1, fast), Baseline: sample(900, 9, fast)}, Advance},
		{"last stage before 100 promotes", Input{Stage: 1, Elapsed: time.Minute, Canary: sample(100, 0, fast), Baseline: sample(100, 0, fast)}, Promote},
		{"error rate breach rolls back before interval", Input{Elapsed: time.Second, Canary: sample(100, 10, fast), Baseline: sample(900, 0, fast)}, Rollback},
		{"error breach ignored under min requests", Input{Elapsed: time.Second, Canary: sample(10, 10, fast), Baseline: sample(900, 0, fast)}, Hold},
		{"p95 breach rolls back", Input{Elapsed: time.Second, Canary: sample(100, 0, slow), Baseline: sample(900, 0, fast)}, Rollback},
		{"p95 under floor is not a breach", Input{Elapsed: time.Second, Canary: sample(100, 0, 1), Baseline: sample(900, 0, 0)}, Hold},
		{"baseline errors raise the bar", Input{Elapsed: time.Minute, Canary: sample(100, 5, fast), Baseline: sample(100, 5, fast)}, Advance},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Policy, tc.in.Limits = testPolicy(), testLimits()
			if got := Decide(tc.in); got.Action != tc.want {
				t.Fatalf("Decide = %s (%s), want %s", got.Action, got.Reason, tc.want)
			}
		})
	}
}

func TestStages(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"10,25,50,100", false}, {"5", false}, {"", false},
		{"25,10", true}, {"0,50", true}, {"100", true}, {"10,x", true}, {"10,10", true}, {"1,2,3,4,5,6,7,8,9,10,11", true},
	}
	for _, tc := range tests {
		if _, err := ParseStages(tc.in); (err != nil) != tc.wantErr {
			t.Errorf("ParseStages(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
	}
	if w := testPolicy().Weight(2); w != MaxWeight {
		t.Fatalf("stage 100 weight = %d, want %d", w, MaxWeight)
	}
}

func TestDefaultPolicyEnv(t *testing.T) {
	env := map[string]string{EnvStages: "5,20", EnvInterval: "30s", EnvMaxErrorRate: "0.1", EnvMaxP95Ratio: "3", EnvMinRequests: "7"}
	get := func(k string) string { return env[k] }
	p := DefaultPolicy(get)
	if len(p.Stages) != 2 || p.Interval != 30*time.Second || p.MaxErrorRate != 0.1 || p.MaxP95Ratio != 3 {
		t.Fatalf("env not applied: %+v", p)
	}
	if DefaultLimits(get).MinRequests != 7 {
		t.Fatal("min requests env not applied")
	}
	if d := DefaultPolicy(func(string) string { return "" }); d.Validate() != nil {
		t.Fatalf("defaults invalid: %v", d.Validate())
	}
}

type fakeStore struct {
	rels    map[string]store.CanaryRelease
	setErr  error
	setCall int
}

func (f *fakeStore) ListCanaryReleases(context.Context) ([]store.CanaryRelease, error) {
	var out []store.CanaryRelease
	for _, r := range f.rels {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeStore) SetCanaryRelease(_ context.Context, c store.CanaryRelease) error {
	f.setCall++
	if f.setErr != nil {
		return f.setErr
	}
	f.rels[c.ServiceName] = c
	return nil
}

type fakeStats struct {
	m map[string]telemetry.UpstreamStat
}

func (f *fakeStats) UpstreamTotals() map[string]telemetry.UpstreamStat { return f.m }

func (f *fakeStats) add(up string, reqs, errs uint64, bucket int) {
	u := f.m[up]
	u.Requests += reqs
	u.Errors5xx += errs
	u.Latency[bucket] += reqs
	f.m[up] = u
}

type fakeActions struct {
	promoteErrs  []error
	rollbackErrs []error
	promoted     int
	rolledBack   []string
	store        *fakeStore
}

func (f *fakeActions) Promote(_ context.Context, rel store.CanaryRelease) error {
	f.promoted++
	if len(f.promoteErrs) > 0 {
		err := f.promoteErrs[0]
		f.promoteErrs = f.promoteErrs[1:]
		if err != nil {
			return err
		}
	}
	delete(f.store.rels, rel.ServiceName)
	return nil
}

func (f *fakeActions) Rollback(_ context.Context, rel store.CanaryRelease, reason string) error {
	f.rolledBack = append(f.rolledBack, reason)
	if len(f.rollbackErrs) > 0 {
		err := f.rollbackErrs[0]
		f.rollbackErrs = f.rollbackErrs[1:]
		if err != nil {
			return err
		}
	}
	delete(f.store.rels, rel.ServiceName)
	return nil
}

type harness struct {
	store *fakeStore
	stats *fakeStats
	acts  *fakeActions
	rec   *Reconciler
	clock time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{clock: time.Unix(1000, 0)}
	h.store = &fakeStore{rels: map[string]store.CanaryRelease{"web": {
		ServiceName: "web", CanaryService: "web--canary", Image: "web:2", Weight: 10,
		Stages: []int{10, 50, 100}, IntervalSecs: 60, MaxErrorRate: 0.02, MaxP95Ratio: 1.5,
	}}}
	h.stats = &fakeStats{m: map[string]telemetry.UpstreamStat{}}
	h.acts = &fakeActions{store: h.store}
	dials := NewDials()
	dials.Set("web", Pair{Stable: "s:1", Canary: "c:1"})
	h.rec = NewReconciler(h.store, h.stats, dials, h.acts, testLimits(), func() time.Time { return h.clock }, nil)
	return h
}

func (h *harness) tick(t *testing.T, advance time.Duration) error {
	t.Helper()
	h.clock = h.clock.Add(advance)
	return h.rec.Reconcile(context.Background())
}

func (h *harness) traffic(canaryErrs uint64) {
	h.stats.add("s:1", 270, 0, 2)
	h.stats.add("c:1", 30, canaryErrs, 2)
}

func TestReconcilerWalksStagesAndPromotes(t *testing.T) {
	h := newHarness(t)
	_ = h.tick(t, 0)
	h.traffic(0)
	_ = h.tick(t, time.Minute)
	if got := h.store.rels["web"]; got.StageIndex != 1 || got.Weight != 50 {
		t.Fatalf("after stage 1: stage %d weight %d", got.StageIndex, got.Weight)
	}
	_ = h.tick(t, time.Second)
	h.traffic(0)
	_ = h.tick(t, time.Minute)
	if h.acts.promoted != 1 {
		t.Fatalf("promoted %d times, want 1", h.acts.promoted)
	}
	if _, ok := h.store.rels["web"]; ok {
		t.Fatal("release should be gone after promote")
	}
}

func TestReconcilerRollsBackOnBreach(t *testing.T) {
	h := newHarness(t)
	_ = h.tick(t, 0)
	h.traffic(10)
	_ = h.tick(t, time.Second)
	if len(h.acts.rolledBack) != 1 || h.acts.promoted != 0 {
		t.Fatalf("rollbacks %v promoted %d", h.acts.rolledBack, h.acts.promoted)
	}
}

func TestReconcilerRollbackHalfSucceededRetries(t *testing.T) {
	h := newHarness(t)
	h.acts.rollbackErrs = []error{errors.New("remove clone failed")}
	_ = h.tick(t, 0)
	h.traffic(10)
	if err := h.tick(t, time.Second); err == nil {
		t.Fatal("first rollback should surface its error")
	}
	if _, ok := h.store.rels["web"]; !ok {
		t.Fatal("release must survive a failed rollback so the next tick retries")
	}
	if err := h.tick(t, time.Second); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if len(h.acts.rolledBack) != 2 {
		t.Fatalf("rollback attempts = %d, want 2", len(h.acts.rolledBack))
	}
	if _, ok := h.store.rels["web"]; ok {
		t.Fatal("release should be gone after the retry")
	}
}

func TestReconcilerPromoteFailureRecordsReasonAndRetriesLater(t *testing.T) {
	h := newHarness(t)
	h.store.rels["web"] = func() store.CanaryRelease {
		r := h.store.rels["web"]
		r.StageIndex, r.Weight = 1, 50
		return r
	}()
	h.acts.promoteErrs = []error{errors.New("deploy refused")}
	_ = h.tick(t, 0)
	h.traffic(0)
	if err := h.tick(t, time.Minute); err == nil {
		t.Fatal("promote error should surface")
	}
	if got := h.store.rels["web"].StatusReason; got != "promote failed: deploy refused" {
		t.Fatalf("status reason = %q", got)
	}
	_ = h.tick(t, time.Second)
	if h.acts.promoted != 1 {
		t.Fatal("must not retry before another interval")
	}
	h.traffic(0)
	_ = h.tick(t, time.Minute)
	if h.acts.promoted != 2 {
		t.Fatalf("promote attempts = %d, want 2", h.acts.promoted)
	}
}

func TestReconcilerAdvanceSaveFailureIsRepeated(t *testing.T) {
	h := newHarness(t)
	_ = h.tick(t, 0)
	h.traffic(0)
	h.store.setErr = errors.New("db busy")
	if err := h.tick(t, time.Minute); err == nil {
		t.Fatal("save failure should surface")
	}
	h.store.setErr = nil
	if err := h.tick(t, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := h.store.rels["web"]; got.StageIndex != 1 {
		t.Fatalf("stage = %d, want 1 after the retry", got.StageIndex)
	}
}

func TestReconcilerIgnoresManualAndPausedAndUnroutedCanaries(t *testing.T) {
	h := newHarness(t)
	h.store.rels["manual"] = store.CanaryRelease{ServiceName: "manual", Weight: 10}
	paused := h.store.rels["web"]
	paused.Weight = 0
	h.store.rels["web"] = paused
	_ = h.tick(t, 0)
	h.traffic(10)
	_ = h.tick(t, time.Hour)
	if len(h.acts.rolledBack) != 0 || h.acts.promoted != 0 {
		t.Fatal("paused or manual canaries must not be acted on")
	}
}
