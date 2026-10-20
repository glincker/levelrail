package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestWithOnAttempt_HTTPReportsEachAttempt confirms the side channel
// fires once per real attempt, with the right outcome and status code,
// and that WaitReady's own retry/error behavior is unaffected by it
// being configured.
func TestWithOnAttempt_HTTPReportsEachAttempt(t *testing.T) {
	var mu sync.Mutex
	var seen []Attempt
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := New(srv.Client(), nil, Limits{}, WithOnAttempt(func(a Attempt) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, a)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.WaitReady(ctx, Target{Addr: hostPort(t, srv)}, Config{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: time.Second})
	if err != nil {
		t.Fatalf("WaitReady() = %v, want nil once the server returns 200", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("onAttempt fired %d times, want exactly 3 (one per real attempt)", len(seen))
	}
	if seen[0].Success || seen[0].StatusCode != http.StatusServiceUnavailable {
		t.Errorf("attempt 1 = %+v, want a failed 503", seen[0])
	}
	if !seen[2].Success || seen[2].StatusCode != http.StatusOK {
		t.Errorf("attempt 3 = %+v, want a successful 200", seen[2])
	}
	for _, a := range seen {
		if a.Target == "" {
			t.Errorf("attempt %+v has an empty Target", a)
		}
		if a.Time.IsZero() {
			t.Errorf("attempt %+v has a zero Time", a)
		}
	}
}

// TestWithOnAttempt_NeverChangesOutcome runs the same WaitReady call with
// and without a recorder configured and asserts the return value and
// attempt count are identical: the side channel must never influence
// WaitReady's own retry or pass/fail logic.
func TestWithOnAttempt_NeverChangesOutcome(t *testing.T) {
	newServer := func() (*httptest.Server, *int) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		return srv, &calls
	}

	srvA, callsA := newServer()
	defer srvA.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	errA := New(srvA.Client(), nil, Limits{}).WaitReady(ctx, Target{Addr: hostPort(t, srvA)}, Config{Path: "/", Interval: 20 * time.Millisecond, Timeout: 500 * time.Millisecond})

	srvB, callsB := newServer()
	defer srvB.Close()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel2()
	var reported int
	errB := New(srvB.Client(), nil, Limits{}, WithOnAttempt(func(Attempt) { reported++ })).
		WaitReady(ctx2, Target{Addr: hostPort(t, srvB)}, Config{Path: "/", Interval: 20 * time.Millisecond, Timeout: 500 * time.Millisecond})

	if (errA == nil) != (errB == nil) {
		t.Fatalf("outcome differs: without recorder err=%v, with recorder err=%v", errA, errB)
	}
	if *callsA != *callsB {
		t.Errorf("attempt count differs: without recorder %d, with recorder %d", *callsA, *callsB)
	}
	if reported != *callsB {
		t.Errorf("onAttempt fired %d times, want exactly %d (one per real attempt)", reported, *callsB)
	}
}

// TestWithOnAttempt_Exec confirms the side channel also fires for exec
// probes, reporting the exit code rather than an HTTP status.
func TestWithOnAttempt_Exec(t *testing.T) {
	exec := &fakeExecutor{code: 1}
	var got Attempt
	p := New(nil, exec, Limits{}, WithOnAttempt(func(a Attempt) { got = a }))

	err := p.Check(context.Background(), Target{ContainerID: "c1"}, Config{Exec: []string{"false"}})
	if err == nil {
		t.Fatal("Check() = nil, want a non-zero-exit failure")
	}
	if got.Success {
		t.Errorf("attempt reported Success = true, want false")
	}
	if got.ExitCode != 1 {
		t.Errorf("attempt ExitCode = %d, want 1", got.ExitCode)
	}
	if got.Target != "c1" {
		t.Errorf("attempt Target = %q, want %q", got.Target, "c1")
	}
	if got.Error == "" {
		t.Errorf("attempt Error is empty, want the failure reason")
	}
}
