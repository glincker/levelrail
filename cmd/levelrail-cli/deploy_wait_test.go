package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// deployStatusServer serves GET /api/v1/apps/{name}/deploys, returning
// responses[call] for the call-th request (0-indexed), clamped to the
// last entry once calls exceed len(responses): the same "running for a
// while, then terminal" shape a real reconcile convergence has.
func deployStatusServer(t *testing.T, responses [][]conditionResource) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1) - 1
		idx := int(n)
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responses[idx])
	}))
	return srv, &calls
}

func fakeDeploySleep(slept *[]time.Duration) func(context.Context, time.Duration) {
	return func(_ context.Context, d time.Duration) {
		*slept = append(*slept, d)
	}
}

func TestWaitForDeployToConverge_EventuallySucceeds(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fresh := since.Add(time.Minute)
	srv, calls := deployStatusServer(t, [][]conditionResource{
		{{Type: "Ready", Status: "Unknown", Reason: "NoDesiredState", LastTransitionTime: fresh}},
		{{Type: "Ready", Status: "Unknown", Reason: "NoDesiredState", LastTransitionTime: fresh}},
		{{Type: "Ready", Status: "True", Reason: "Deployed", LastTransitionTime: fresh.Add(time.Second)}},
	})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var slept []time.Duration
	var progress bytes.Buffer

	outcome, err := waitForDeployToConverge(context.Background(), client, "web", since, time.Hour, fakeDeploySleep(&slept), &progress)
	if err != nil {
		t.Fatalf("waitForDeployToConverge() error = %v", err)
	}
	if !outcome.Succeeded {
		t.Errorf("outcome.Succeeded = false, want true (reason=%q message=%q)", outcome.Reason, outcome.Message)
	}
	if outcome.Reason != "Deployed" {
		t.Errorf("outcome.Reason = %q, want %q", outcome.Reason, "Deployed")
	}
	if got := int(atomic.LoadInt32(calls)); got != 3 {
		t.Errorf("status checks = %d, want 3", got)
	}
	if len(slept) != 2 {
		t.Errorf("sleep calls = %d, want 2 (once between each non-terminal check)", len(slept))
	}
	if progress.Len() == 0 {
		t.Errorf("progress writer got no output, want a dot per non-terminal check")
	}
}

func TestWaitForDeployToConverge_EventuallyFails(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fresh := since.Add(time.Minute)
	srv, _ := deployStatusServer(t, [][]conditionResource{
		{{Type: "Ready", Status: "False", Reason: "InspectFailed", Message: "container exited immediately", LastTransitionTime: fresh}},
	})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var slept []time.Duration

	outcome, err := waitForDeployToConverge(context.Background(), client, "web", since, time.Hour, fakeDeploySleep(&slept), nil)
	if err != nil {
		t.Fatalf("waitForDeployToConverge() error = %v", err)
	}
	if outcome.Succeeded {
		t.Errorf("outcome.Succeeded = true, want false")
	}
	if outcome.Reason != "InspectFailed" || outcome.Message != "container exited immediately" {
		t.Errorf("outcome = %+v, want reason InspectFailed with the container-exit message", outcome)
	}
	if len(slept) != 0 {
		t.Errorf("sleep calls = %d, want 0 (first check was already terminal)", len(slept))
	}
}

// TestWaitForDeployToConverge_IgnoresStaleReadyCondition covers the
// whole reason this function needs a baseline at all: a Ready condition
// left over from a previous, already-successful deploy must not be
// mistaken for this one's own convergence.
func TestWaitForDeployToConverge_IgnoresStaleReadyCondition(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stale := since.Add(-time.Minute)
	fresh := since.Add(time.Minute)
	srv, calls := deployStatusServer(t, [][]conditionResource{
		{{Type: "Ready", Status: "True", Reason: "Deployed", LastTransitionTime: stale}},
		{{Type: "Ready", Status: "True", Reason: "Deployed", LastTransitionTime: fresh}},
	})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var slept []time.Duration

	outcome, err := waitForDeployToConverge(context.Background(), client, "web", since, time.Hour, fakeDeploySleep(&slept), nil)
	if err != nil {
		t.Fatalf("waitForDeployToConverge() error = %v", err)
	}
	if !outcome.Succeeded {
		t.Errorf("outcome.Succeeded = false, want true once a fresh Ready=True arrives")
	}
	if got := int(atomic.LoadInt32(calls)); got != 2 {
		t.Errorf("status checks = %d, want 2 (first was stale and must be skipped)", got)
	}
}

func TestWaitForDeployToConverge_TimesOut(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fresh := since.Add(time.Minute)
	srv, _ := deployStatusServer(t, [][]conditionResource{
		{{Type: "Ready", Status: "Unknown", Reason: "NoDesiredState", LastTransitionTime: fresh}},
	})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var slept []time.Duration

	_, err := waitForDeployToConverge(context.Background(), client, "web", since, time.Nanosecond, fakeDeploySleep(&slept), nil)
	if !errors.Is(err, errDeployWaitTimeout) {
		t.Fatalf("waitForDeployToConverge() error = %v, want errDeployWaitTimeout", err)
	}
}

func TestWaitForDeployToConverge_RespectsCancelledContext(t *testing.T) {
	srv, calls := deployStatusServer(t, [][]conditionResource{
		{{Type: "Ready", Status: "True", Reason: "Deployed", LastTransitionTime: time.Now()}},
	})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var slept []time.Duration
	_, err := waitForDeployToConverge(ctx, client, "web", time.Time{}, time.Hour, fakeDeploySleep(&slept), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForDeployToConverge() error = %v, want context.Canceled", err)
	}
	if got := int(atomic.LoadInt32(calls)); got != 0 {
		t.Errorf("status checks = %d, want 0 (a cancelled context must never even poll once)", got)
	}
	if len(slept) != 0 {
		t.Errorf("sleep calls = %d, want 0", len(slept))
	}
}

func TestDeployReadySince(t *testing.T) {
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	srv, _ := deployStatusServer(t, [][]conditionResource{
		{{Type: "Ready", Status: "True", Reason: "Deployed", LastTransitionTime: want}},
	})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var stderr bytes.Buffer
	got := deployReadySince(context.Background(), client, "web", &stderr)
	if !got.Equal(want) {
		t.Errorf("deployReadySince() = %v, want %v", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want no warning on a successful read", stderr.String())
	}
}

func TestDeployReadySince_NoStoredCondition(t *testing.T) {
	srv, _ := deployStatusServer(t, [][]conditionResource{{}})
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var stderr bytes.Buffer
	got := deployReadySince(context.Background(), client, "web", &stderr)
	if !got.IsZero() {
		t.Errorf("deployReadySince() = %v, want the zero time when no Ready condition is stored yet", got)
	}
}

func TestDeployReadySince_ServerErrorFallsBackToZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var stderr bytes.Buffer
	got := deployReadySince(context.Background(), client, "web", &stderr)
	if !got.IsZero() {
		t.Errorf("deployReadySince() = %v, want the zero time on a status-check failure", got)
	}
	if !strings.Contains(stderr.String(), "warning") {
		t.Errorf("stderr = %q, want a warning about the failed pre-check", stderr.String())
	}
}
