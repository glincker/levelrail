package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustParseRFC3339(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return tm
}

func TestComputeRolloutOutcome(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	before := mustParseRFC3339(t, "2025-12-31T00:00:00Z")
	after := mustParseRFC3339(t, "2026-01-01T00:00:10Z")

	tests := []struct {
		name       string
		attempt    deployAttemptResource
		conditions []conditionResource
		wantState  string
	}{
		{
			name:      "build itself failed",
			attempt:   deployAttemptResource{Status: "failed"},
			wantState: "failed",
		},
		{
			name:      "still building",
			attempt:   deployAttemptResource{Status: "running"},
			wantState: "pending",
		},
		{
			name:      "succeeded but not yet finished (defensive)",
			attempt:   deployAttemptResource{Status: "succeeded", FinishedAt: nil},
			wantState: "pending",
		},
		{
			name:       "Deployed after finished_at",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "Deployed", LastTransitionTime: after}},
			wantState:  "succeeded",
		},
		{
			name:       "AlreadyRunning after finished_at",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "AlreadyRunning", LastTransitionTime: after}},
			wantState:  "succeeded",
		},
		{
			name:       "done condition predates finished_at: still pending",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "AlreadyRunning", LastTransitionTime: before}},
			wantState:  "pending",
		},
		{
			name:       "ReadinessFailed after finished_at",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "ReadinessFailed", LastTransitionTime: after}},
			wantState:  "failed",
		},
		{
			name:       "OOMKilledDuringReadiness after finished_at",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "OOMKilledDuringReadiness", LastTransitionTime: after}},
			wantState:  "failed",
		},
		{
			name:       "ExitedDuringReadiness after finished_at",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "ExitedDuringReadiness", LastTransitionTime: after}},
			wantState:  "failed",
		},
		{
			name:       "unrelated condition: still pending",
			attempt:    deployAttemptResource{Status: "succeeded", FinishedAt: &finished},
			conditions: []conditionResource{{Reason: "Deploying", LastTransitionTime: after}},
			wantState:  "pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeRolloutOutcome(tt.attempt, tt.conditions)
			if got.state != tt.wantState {
				t.Errorf("computeRolloutOutcome() state = %q, want %q", got.state, tt.wantState)
			}
		})
	}
}

// fakeDeployAttemptFetcher lets waitForRollout tests script a sequence of
// responses without a real HTTP server, one entry consumed per poll.
type fakeDeployAttemptFetcher struct {
	calls      atomic.Int32
	attempts   [][]deployAttemptResource
	conditions [][]conditionResource
	err        error
}

func (f *fakeDeployAttemptFetcher) ListDeployAttempts(_ context.Context, _ string) ([]deployAttemptResource, error) {
	if f.err != nil {
		return nil, f.err
	}
	i := int(f.calls.Load())
	if i >= len(f.attempts) {
		i = len(f.attempts) - 1
	}
	return f.attempts[i], nil
}

func (f *fakeDeployAttemptFetcher) GetDeployStatus(_ context.Context, _ string) ([]conditionResource, error) {
	i := int(f.calls.Add(1)) - 1
	if i >= len(f.conditions) {
		i = len(f.conditions) - 1
	}
	return f.conditions[i], nil
}

func TestWaitForRollout_SucceedsOnASubsequentPoll(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	after := mustParseRFC3339(t, "2026-01-01T00:00:10Z")
	attempt := deployAttemptResource{ID: "dep_1", Status: "succeeded", FinishedAt: &finished}

	fake := &fakeDeployAttemptFetcher{
		attempts: [][]deployAttemptResource{{attempt}},
		conditions: [][]conditionResource{
			{{Reason: "Deploying", LastTransitionTime: after}},
			{{Reason: "Deploying", LastTransitionTime: after}},
			{{Reason: "AlreadyRunning", LastTransitionTime: after}},
		},
	}

	var ticks int
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := waitForRollout(ctx, fake, rolloutWaitConfig{Name: "web", AttemptID: "", PollInterval: 5 * time.Millisecond, OnTick: func(rolloutOutcome) { ticks++ }})
	if err != nil {
		t.Fatalf("waitForRollout() error = %v", err)
	}
	if outcome.state != "succeeded" {
		t.Errorf("outcome.state = %q, want succeeded", outcome.state)
	}
	if ticks != 3 {
		t.Errorf("ticks = %d, want 3 (pending, pending, succeeded)", ticks)
	}
}

func TestWaitForRollout_SpecificAttemptID(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	after := mustParseRFC3339(t, "2026-01-01T00:00:10Z")
	fake := &fakeDeployAttemptFetcher{
		attempts: [][]deployAttemptResource{{
			{ID: "dep_2", Status: "running"},
			{ID: "dep_1", Status: "succeeded", FinishedAt: &finished},
		}},
		conditions: [][]conditionResource{{{Reason: "Deployed", LastTransitionTime: after}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	outcome, err := waitForRollout(ctx, fake, rolloutWaitConfig{Name: "web", AttemptID: "dep_1", PollInterval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("waitForRollout() error = %v", err)
	}
	if outcome.state != "succeeded" {
		t.Errorf("outcome.state = %q, want succeeded (should target dep_1, not the newer dep_2)", outcome.state)
	}
}

func TestWaitForRollout_AttemptIDNotFound(t *testing.T) {
	fake := &fakeDeployAttemptFetcher{
		attempts: [][]deployAttemptResource{{{ID: "dep_1", Status: "succeeded"}}},
	}
	_, err := waitForRollout(context.Background(), fake, rolloutWaitConfig{Name: "web", AttemptID: "dep_ghost", PollInterval: time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("waitForRollout() error = %v, want a not-found error", err)
	}
}

func TestWaitForRollout_ContextTimeout(t *testing.T) {
	fake := &fakeDeployAttemptFetcher{
		attempts:   [][]deployAttemptResource{{{ID: "dep_1", Status: "running"}}},
		conditions: [][]conditionResource{{}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	outcome, err := waitForRollout(ctx, fake, rolloutWaitConfig{Name: "web", AttemptID: "", PollInterval: 5 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waitForRollout() error = %v, want context.DeadlineExceeded", err)
	}
	if outcome.state != "pending" {
		t.Errorf("outcome.state = %q, want pending on timeout", outcome.state)
	}
}

func TestWaitForRollout_ListError(t *testing.T) {
	fake := &fakeDeployAttemptFetcher{err: errors.New("network down")}
	_, err := waitForRollout(context.Background(), fake, rolloutWaitConfig{Name: "web", AttemptID: "", PollInterval: time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Errorf("waitForRollout() error = %v, want the underlying error wrapped", err)
	}
}

func TestRun_AppsWait_SucceedsImmediately(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	after := mustParseRFC3339(t, "2026-01-01T00:00:10Z")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "/deploy-attempts"):
			_ = json.NewEncoder(w).Encode([]deployAttemptResource{
				{ID: "dep_1", Status: "succeeded", FinishedAt: &finished},
			})
		case strings.HasSuffix(r.URL.Path, "/deploys"):
			_ = json.NewEncoder(w).Encode([]conditionResource{
				{Reason: "AlreadyRunning", LastTransitionTime: after},
			})
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "wait", "web", "--api-url", srv.URL, "--poll-interval", "5ms", "--timeout", "2s"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"web" rolled out`) {
		t.Errorf("stdout = %q, want a first deploy reported as rolled out", stdout.String())
	}
}

func TestRun_AppsWait_FailsOnConvergedFailure(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	after := mustParseRFC3339(t, "2026-01-01T00:00:10Z")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "/deploy-attempts"):
			_ = json.NewEncoder(w).Encode([]deployAttemptResource{
				{ID: "dep_1", Status: "succeeded", FinishedAt: &finished},
			})
		case strings.HasSuffix(r.URL.Path, "/deploys"):
			_ = json.NewEncoder(w).Encode([]conditionResource{
				{Reason: "ReadinessFailed", Message: "probe never became ready", LastTransitionTime: after},
			})
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "wait", "web", "--api-url", srv.URL, "--poll-interval", "5ms", "--timeout", "2s"}, &stdout, &stderr, envMap())
	if got != exitDeployFailed {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitDeployFailed, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "failed to converge") {
		t.Errorf("stdout = %q, want a failure message", stdout.String())
	}
}

func TestRun_AppsWait_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "/deploy-attempts"):
			_ = json.NewEncoder(w).Encode([]deployAttemptResource{{ID: "dep_1", Status: "running"}})
		case strings.HasSuffix(r.URL.Path, "/deploys"):
			_ = json.NewEncoder(w).Encode([]conditionResource{})
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "wait", "web", "--api-url", srv.URL, "--poll-interval", "5ms", "--timeout", "30ms"}, &stdout, &stderr, envMap())
	if got != exitDeployTimeout {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitDeployTimeout, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "timed out") {
		t.Errorf("stdout = %q, want a timeout message", stdout.String())
	}
}

func TestRun_AppsWait_InvalidTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "wait", "web", "--timeout", "not-a-duration"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_AppsWait_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "wait", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps wait") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func TestSummarizeRollout(t *testing.T) {
	finished := mustParseRFC3339(t, "2026-01-01T00:00:05Z")
	restartAt := "2026-01-01T00:01:00Z"
	older := mustParseRFC3339(t, "2025-12-31T00:00:00Z")
	att := func(id, image, digest string) deployAttemptResource {
		return deployAttemptResource{ID: id, Image: image, ImageDigest: digest, Status: "succeeded", FinishedAt: &finished}
	}
	tests := []struct {
		name     string
		attempts []deployAttemptResource
		target   int
		cond     *conditionResource
		timeline []apiclient.TimelineItem
		want     string
	}{
		{"fresh Deployed reason", []deployAttemptResource{att("b", "web:2", "sha256:2"), att("a", "web:1", "sha256:1")}, 0, &conditionResource{Reason: "Deployed"}, nil, rolloutRolledOut},
		{"AlreadyRunning after a real change", []deployAttemptResource{att("b", "web:2", "sha256:2"), att("a", "web:1", "sha256:1")}, 0, &conditionResource{Reason: "AlreadyRunning"}, nil, rolloutRolledOut},
		{"redeploy of identical digest", []deployAttemptResource{att("b", "web:1", "sha256:1"), att("a", "web:1", "sha256:1")}, 0, &conditionResource{Reason: "AlreadyRunning"}, nil, rolloutUpToDate},
		{"identical tag without digests", []deployAttemptResource{att("b", "web:1", ""), att("a", "web:1", "")}, 0, &conditionResource{Reason: "AlreadyRunning"}, nil, rolloutUpToDate},
		{"first ever deploy", []deployAttemptResource{att("a", "web:1", "sha256:1")}, 0, &conditionResource{Reason: "AlreadyRunning"}, nil, rolloutRolledOut},
		{"restart after the attempt", []deployAttemptResource{att("b", "web:1", "sha256:1"), att("a", "web:1", "sha256:1")}, 0, &conditionResource{Reason: "AlreadyRunning"}, []apiclient.TimelineItem{{Kind: "restart", At: restartAt}}, rolloutRestarted},
		{"restart before the attempt is ignored", []deployAttemptResource{att("b", "web:2", "sha256:2"), att("a", "web:1", "sha256:1")}, 0, &conditionResource{Reason: "AlreadyRunning"}, []apiclient.TimelineItem{{Kind: "restart", At: older.Format(time.RFC3339)}}, rolloutRolledOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeRollout(tt.attempts, tt.attempts[tt.target], tt.cond, tt.timeline); got != tt.want {
				t.Fatalf("summarizeRollout() = %q, want %q", got, tt.want)
			}
		})
	}
}
