package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeProbeAttempts struct {
	mu       sync.Mutex
	attempts []store.ProbeAttempt
}

func (f *fakeProbeAttempts) RecordProbeAttempt(_ context.Context, a store.ProbeAttempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, a)
	return nil
}

func (f *fakeProbeAttempts) all() []store.ProbeAttempt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.ProbeAttempt(nil), f.attempts...)
}

// TestController_Reconcile_RecordsProbeAttempts confirms a configured
// ProbeAttemptRecorder sees one entry per real HTTP attempt waitReady
// makes, with the detail a reconcile condition's own Reason/Message
// never carries (status code, latency), while the deploy's own outcome
// is unaffected by the recorder being present.
func TestController_Reconcile_RecordsProbeAttempts(t *testing.T) {
	srv := healthyAfter(30 * time.Millisecond)
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 200 * time.Millisecond}},
	}
	rec := &fakeProbeAttempts{}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second), WithProbeAttemptRecorder(rec))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Fatalf("condition = %+v, want Status=True Reason=Deployed", cond)
	}

	attempts := rec.all()
	if len(attempts) < 2 {
		t.Fatalf("recorded %d probe attempts, want at least 2 (one failing, one succeeding)", len(attempts))
	}
	last := attempts[len(attempts)-1]
	if !last.Success || last.StatusCode != 200 {
		t.Errorf("last attempt = %+v, want a successful 200", last)
	}
	if attempts[0].Success || attempts[0].StatusCode != 503 {
		t.Errorf("first attempt = %+v, want a failed 503", attempts[0])
	}
	for _, a := range attempts {
		if a.Target == "" {
			t.Errorf("attempt %+v has an empty Target", a)
		}
		if a.ServiceName != "web" || a.Image != "img:v1" {
			t.Errorf("attempt %+v has the wrong resolution key, want ServiceName=web Image=img:v1", a)
		}
	}
}

// TestController_Reconcile_WithProbeAttemptRecorder_SameFailureOutcome
// is TestController_Reconcile_FreshDeploy_ReadinessFails's own case,
// with a ProbeAttemptRecorder configured: the deploy's outcome (error,
// condition reason) must be identical, since recording is a pure side
// channel that runs alongside waitReady's decision, never inside it.
func TestController_Reconcile_WithProbeAttemptRecorder_SameFailureOutcome(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	rec := &fakeProbeAttempts{}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(150*time.Millisecond), WithProbeAttemptRecorder(rec))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a readiness timeout error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "ReadinessFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=ReadinessFailed", cond)
	}
	if len(rec.all()) == 0 {
		t.Error("recorder saw 0 attempts, want at least 1 from the failed probe loop")
	}
}
