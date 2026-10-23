package reconcile

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// flipController returns whatever reason it is currently told to.
type flipController struct {
	name   string
	mu     sync.Mutex
	reason string
}

func (f *flipController) Name() string { return f.name }

func (f *flipController) set(reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reason = reason
}

func (f *flipController) Reconcile(_ context.Context) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Result{Conditions: []Condition{{Type: ConditionTypeReady, Status: ConditionTrue, Reason: f.reason}}}, nil
}

func engineWithClock(c Controller, st Store) (*Engine, *time.Time) {
	e := NewEngine(testLogger(), c)
	e.SetStore(st)
	e.SetFullResync(5 * time.Minute)
	now := time.Now()
	e.now = func() time.Time { return now }
	return e, &now
}

func TestEngine_UnchangedStatusIsNotRewrittenEveryPass(t *testing.T) {
	st := &fakeStore{}
	e, _ := engineWithClock(&flipController{name: "a", reason: "OK"}, st)

	for i := 0; i < 10; i++ {
		e.ReconcileAll(context.Background())
	}
	if got := st.calls(); got != 1 {
		t.Errorf("10 passes with an unchanged status wrote %d times, want 1", got)
	}
}

func TestEngine_ChangedStatusIsWrittenImmediately(t *testing.T) {
	st := &fakeStore{}
	c := &flipController{name: "a", reason: "OK"}
	e, _ := engineWithClock(c, st)

	e.ReconcileAll(context.Background())
	c.set("Degraded")
	e.ReconcileAll(context.Background())
	e.ReconcileAll(context.Background())

	if got := st.calls(); got != 2 {
		t.Errorf("writes = %d, want 2 (first, then the change)", got)
	}
	if st.lastCond[0].Reason != "Degraded" {
		t.Errorf("last written reason = %q, want Degraded", st.lastCond[0].Reason)
	}
}

func TestEngine_FullResyncRewritesAnUnchangedStatus(t *testing.T) {
	st := &fakeStore{}
	e, now := engineWithClock(&flipController{name: "a", reason: "OK"}, st)

	e.ReconcileAll(context.Background())
	*now = now.Add(4 * time.Minute)
	e.ReconcileAll(context.Background())
	if got := st.calls(); got != 1 {
		t.Fatalf("writes before the resync is due = %d, want 1", got)
	}
	*now = now.Add(2 * time.Minute)
	e.ReconcileAll(context.Background())
	if got := st.calls(); got != 2 {
		t.Errorf("writes after the resync is due = %d, want 2", got)
	}
}

func TestEngine_FailedWriteIsRetriedNextPass(t *testing.T) {
	st := &fakeStore{err: errors.New("database is locked")}
	e, _ := engineWithClock(&flipController{name: "a", reason: "OK"}, st)

	e.ReconcileAll(context.Background())
	e.ReconcileAll(context.Background())
	if got := st.calls(); got != 2 {
		t.Errorf("writes while the store fails = %d, want 2 (a failed write is never remembered as done)", got)
	}
	st.mu.Lock()
	st.err = nil
	st.mu.Unlock()
	e.ReconcileAll(context.Background())
	e.ReconcileAll(context.Background())
	if got := st.calls(); got != 3 {
		t.Errorf("writes after recovery = %d, want 3", got)
	}
}

func TestEngine_RemovedControllerIsForgotten(t *testing.T) {
	st := &fakeStore{}
	c := &flipController{name: "app", reason: "OK"}
	present := true
	e := NewEngine(testLogger())
	e.SetStore(st)
	e.SetSource(func(context.Context) ([]Controller, error) {
		if present {
			return []Controller{c}, nil
		}
		return nil, nil
	})

	e.ReconcileAll(context.Background())
	present = false
	e.ReconcileAll(context.Background())
	present = true
	e.ReconcileAll(context.Background())

	if got := st.calls(); got != 2 {
		t.Errorf("writes = %d, want 2: an app deleted and recreated must not inherit the old ledger entry", got)
	}
}

func TestFullResyncFromEnv(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	if got := FullResyncFromEnv(env("90s")); got != 90*time.Second {
		t.Errorf("90s -> %v", got)
	}
	for _, bad := range []string{"", "x", "0", "-5m"} {
		if got := FullResyncFromEnv(env(bad)); got != defaultFullResync {
			t.Errorf("%q -> %v, want default", bad, got)
		}
	}
}

// driftController repairs drift on the first Reconcile after it is flagged.
type driftController struct {
	drifted  atomic.Bool
	repaired atomic.Int32
}

func (d *driftController) Name() string { return "drift" }

func (d *driftController) Reconcile(_ context.Context) (Result, error) {
	if d.drifted.CompareAndSwap(true, false) {
		d.repaired.Add(1)
		return Result{Conditions: []Condition{{Type: ConditionTypeReady, Status: ConditionTrue, Reason: "Repaired"}}}, nil
	}
	return Result{Conditions: []Condition{{Type: ConditionTypeReady, Status: ConditionTrue, Reason: "OK"}}}, nil
}

func TestEngine_Run_ResyncTickCorrectsDriftThatNoEventAnnounced(t *testing.T) {
	c := &driftController{}
	st := &fakeStore{}
	e := NewEngine(testLogger(), c)
	e.SetStore(st)
	e.SetFullResync(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := make(chan docker.Event)
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, events, 20*time.Millisecond) }()

	time.Sleep(60 * time.Millisecond)
	c.drifted.Store(true)

	deadline := time.After(time.Second)
	for c.repaired.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("the resync tick never repaired drift that no event announced")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done

	if got := st.calls(); got < 2 {
		t.Errorf("status writes = %d, want the repair persisted on top of the first write even with a long full resync", got)
	}
}
