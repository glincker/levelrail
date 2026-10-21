package alerting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// KindDeployFailed is platform-wide: it fires while any app has an unresolved failed deploy (fed by DeployDispatcher).
const KindDeployFailed Kind = "deploy_failed"

// DefaultDeployFailedWindow is how long a failed deploy keeps a deploy_failed rule firing when RestartWindow is unset.
const DefaultDeployFailedWindow = time.Hour

// DeployFailure is the latest failed deploy of one app that no later success has cleared.
type DeployFailure struct {
	AppName string
	Image   string
	Error   string
	At      time.Time
}

// DeployFailureSource lists unresolved deploy failures newer than since.
type DeployFailureSource interface {
	RecentDeployFailures(since time.Time) []DeployFailure
}

// DeployOutcomeTracker remembers each app's unresolved deploy failure, fed from DeployDispatcher.Dispatch (in memory, cleared by a success or a restart).
type DeployOutcomeTracker struct {
	mu       sync.Mutex
	failures map[string]DeployFailure
}

// NewDeployOutcomeTracker returns an empty tracker.
func NewDeployOutcomeTracker() *DeployOutcomeTracker {
	return &DeployOutcomeTracker{failures: map[string]DeployFailure{}}
}

// Record stores a failure or clears the app's earlier one on success.
func (t *DeployOutcomeTracker) Record(ev DeployOutcome, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ev.Succeeded {
		delete(t.failures, ev.AppName)
		return
	}
	t.failures[ev.AppName] = DeployFailure{AppName: ev.AppName, Image: ev.Image, Error: ev.Error, At: at}
}

// RecentDeployFailures implements DeployFailureSource, ordered by app name.
func (t *DeployOutcomeTracker) RecentDeployFailures(since time.Time) []DeployFailure {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []DeployFailure
	for _, f := range t.failures {
		if !f.At.Before(since) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AppName < out[j].AppName })
	return out
}

// EvaluateDeployFailed fires while any app's latest deploy failed within the window (RestartWindow, default one hour) and has not since succeeded.
func EvaluateDeployFailed(_ context.Context, src DeployFailureSource, r Rule, now time.Time) (Rule, string, error) {
	window := r.RestartWindow
	if window <= 0 {
		window = DefaultDeployFailedWindow
	}
	next := r
	next.LastEvaluatedAt = &now
	failures := src.RecentDeployFailures(now.Add(-window))
	v := float64(len(failures))
	next.LastValue = &v
	next = advanceState(next, r, len(failures) > 0, r.ForDuration, now)
	if len(failures) == 0 {
		return next, "", nil
	}
	parts := make([]string, 0, len(failures))
	for _, f := range failures {
		s := fmt.Sprintf("%s -> %s", f.AppName, f.Image)
		if f.Error != "" {
			s += ": " + f.Error
		}
		parts = append(parts, s)
	}
	return next, "deploy failed: " + strings.Join(parts, "; "), nil
}
