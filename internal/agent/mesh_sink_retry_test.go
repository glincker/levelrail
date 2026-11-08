package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

// scriptedMeshTransport returns errs in order, then succeeds; a nil entry blocks until the attempt's context ends.
type scriptedMeshTransport struct {
	Transport
	mu    sync.Mutex
	errs  []error
	block bool
	calls int
}

func (f *scriptedMeshTransport) ApplyMesh(ctx context.Context, _ network.DeviceConfig) (network.NodeIdentity, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return network.NodeIdentity{}, ctx.Err()
	}
	if n <= len(f.errs) {
		return network.NodeIdentity{}, f.errs[n-1]
	}
	return network.NodeIdentity{}, nil
}

func (f *scriptedMeshTransport) RotateMeshKey(context.Context) (network.RotationResult, error) {
	return network.RotationResult{}, nil
}

type fakeSleeper struct{ waits []time.Duration }

func (s *fakeSleeper) sleep(_ context.Context, d time.Duration) error {
	s.waits = append(s.waits, d)
	return nil
}

func policyFor(timeout, backoff time.Duration, s *fakeSleeper) ApplyPolicy {
	return ApplyPolicy{
		Timeout: func() time.Duration { return timeout },
		Retries: func() int { return 3 },
		Backoff: func() time.Duration { return backoff },
		Sleep:   s.sleep,
	}
}

func TestGRPCSinkApplyMeshRetriesSessionClosedWithBackoff(t *testing.T) {
	registry := NewRegistry()
	tr := &scriptedMeshTransport{errs: []error{ErrSessionClosed, ErrSessionClosed}}
	registry.Register("node-a", tr)
	s := &fakeSleeper{}
	sink := NewGRPCSink(registry, WithApplyPolicy(policyFor(time.Second, 100*time.Millisecond, s)))

	if _, err := sink.ApplyMesh(context.Background(), "node-a", network.DeviceConfig{}); err != nil {
		t.Fatalf("ApplyMesh: %v", err)
	}
	if tr.calls != 3 {
		t.Errorf("calls = %d, want 3", tr.calls)
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if len(s.waits) != len(want) || s.waits[0] != want[0] || s.waits[1] != want[1] {
		t.Errorf("backoff waits = %v, want %v", s.waits, want)
	}
}

func TestGRPCSinkApplyMeshBackoffCappedAtTimeout(t *testing.T) {
	registry := NewRegistry()
	tr := &scriptedMeshTransport{errs: []error{ErrSessionClosed, ErrSessionClosed, ErrSessionClosed}}
	registry.Register("node-a", tr)
	s := &fakeSleeper{}
	sink := NewGRPCSink(registry, WithApplyPolicy(policyFor(300*time.Millisecond, 200*time.Millisecond, s)))

	if _, err := sink.ApplyMesh(context.Background(), "node-a", network.DeviceConfig{}); err != nil {
		t.Fatalf("ApplyMesh: %v", err)
	}
	if got := s.waits[len(s.waits)-1]; got != 300*time.Millisecond {
		t.Errorf("last wait = %v, want capped at 300ms (all waits %v)", got, s.waits)
	}
}

func TestGRPCSinkApplyMeshDoesNotRetryPermanentError(t *testing.T) {
	registry := NewRegistry()
	tr := &scriptedMeshTransport{errs: []error{errors.New("device apply failed")}}
	registry.Register("node-a", tr)
	s := &fakeSleeper{}
	sink := NewGRPCSink(registry, WithApplyPolicy(policyFor(time.Second, time.Millisecond, s)))

	if _, err := sink.ApplyMesh(context.Background(), "node-a", network.DeviceConfig{}); err == nil {
		t.Fatal("ApplyMesh succeeded, want the device error")
	}
	if tr.calls != 1 || len(s.waits) != 0 {
		t.Errorf("calls = %d waits = %v, want one call and no retry", tr.calls, s.waits)
	}
}

func TestGRPCSinkApplyMeshDoesNotRetryAfterDeadline(t *testing.T) {
	registry := NewRegistry()
	tr := &scriptedMeshTransport{block: true}
	registry.Register("node-a", tr)
	s := &fakeSleeper{}
	sink := NewGRPCSink(registry, WithApplyPolicy(policyFor(20*time.Millisecond, time.Millisecond, s)))

	start := time.Now()
	_, err := sink.ApplyMesh(context.Background(), "node-a", network.DeviceConfig{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ApplyMesh error = %v, want DeadlineExceeded", err)
	}
	// The timed out push may still land on the agent, so a resend could apply stale config.
	if tr.calls != 1 || len(s.waits) != 0 {
		t.Errorf("calls = %d waits = %v, want one attempt and no retry", tr.calls, s.waits)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("ApplyMesh took %v, want bounded by the per-attempt timeout", elapsed)
	}
}

func TestGRPCSinkApplyMeshNoRetryWhenCallerContextDone(t *testing.T) {
	registry := NewRegistry()
	tr := &scriptedMeshTransport{errs: []error{ErrSessionClosed}}
	registry.Register("node-a", tr)
	s := &fakeSleeper{}
	sink := NewGRPCSink(registry, WithApplyPolicy(policyFor(time.Second, time.Millisecond, s)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := sink.ApplyMesh(ctx, "node-a", network.DeviceConfig{}); err == nil {
		t.Fatal("ApplyMesh succeeded on a cancelled context")
	}
	if tr.calls != 1 {
		t.Errorf("calls = %d, want 1", tr.calls)
	}
}

func TestRegistryChangeHookFiresOnRegisterAndUnregister(t *testing.T) {
	registry := NewRegistry()
	var got []string
	registry.SetChangeHook(func(id string) { got = append(got, id) })
	registry.Register("node-a", &scriptedMeshTransport{})
	registry.Unregister("node-a")
	if len(got) != 2 || got[0] != "node-a" || got[1] != "node-a" {
		t.Errorf("hook calls = %v, want node-a twice", got)
	}
}
