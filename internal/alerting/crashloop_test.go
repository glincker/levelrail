package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRestartTracker_FirstStart_NotCountedAsRestart(t *testing.T) {
	tr := NewRestartTracker()
	now := time.Now()
	tr.Observe("service:web", "web-abc12345", now)

	if got := tr.CountSince("service:web", now.Add(-time.Hour)); got != 0 {
		t.Errorf("CountSince() = %d, want 0 (a container's first-ever start is not a restart)", got)
	}
}

func TestRestartTracker_SubsequentStarts_CountAsRestarts(t *testing.T) {
	tr := NewRestartTracker()
	base := time.Now()
	tr.Observe("service:web", "web-abc12345", base)                    // first: not counted
	tr.Observe("service:web", "web-abc12345", base.Add(time.Minute))   // restart 1
	tr.Observe("service:web", "web-abc12345", base.Add(2*time.Minute)) // restart 2

	if got := tr.CountSince("service:web", base.Add(-time.Hour)); got != 2 {
		t.Errorf("CountSince() = %d, want 2", got)
	}
}

func TestRestartTracker_NewContainerName_FirstStartNotCounted(t *testing.T) {
	// A redeploy produces a new container name (different image hash):
	// its first start must not be counted as a restart, even though the
	// same resourceID already has restart history from the old name.
	tr := NewRestartTracker()
	base := time.Now()
	tr.Observe("service:web", "web-oldhash1", base)
	tr.Observe("service:web", "web-oldhash1", base.Add(time.Minute)) // 1 restart on the old container

	tr.Observe("service:web", "web-newhash2", base.Add(2*time.Minute)) // new deploy: first start of a new name

	if got := tr.CountSince("service:web", base.Add(-time.Hour)); got != 1 {
		t.Errorf("CountSince() = %d, want 1 (only the old container's real restart, not the new deploy's first start)", got)
	}
}

func TestRestartTracker_CountSince_RespectsWindow(t *testing.T) {
	tr := NewRestartTracker()
	base := time.Now()
	tr.Observe("service:web", "web-abc12345", base)
	tr.Observe("service:web", "web-abc12345", base.Add(time.Minute))    // inside a 5m window from base+10m
	tr.Observe("service:web", "web-abc12345", base.Add(20*time.Minute)) // outside if we ask relative to base+10m-5m..base+10m

	if got := tr.CountSince("service:web", base.Add(9*time.Minute)); got != 1 {
		t.Errorf("CountSince(base+9m) = %d, want 1 (only base+20m falls after that cutoff)", got)
	}
}

func TestRestartTracker_Prune_DropsOldEntriesOnly(t *testing.T) {
	tr := NewRestartTracker()
	base := time.Now()
	tr.Observe("service:web", "web-abc12345", base)
	tr.Observe("service:web", "web-abc12345", base.Add(time.Minute)) // old, should be pruned
	tr.Observe("service:web", "web-abc12345", base.Add(2*time.Hour)) // recent, should stay

	tr.Prune(base.Add(time.Hour))

	if got := tr.CountSince("service:web", base.Add(-24*time.Hour)); got != 1 {
		t.Errorf("CountSince() after Prune = %d, want 1 (only the recent restart survives)", got)
	}
}

func TestResolveResourceID(t *testing.T) {
	services := []store.DesiredService{{Name: "web"}, {Name: "web-worker"}}

	tests := []struct {
		name          string
		containerName string
		wantID        string
		wantOK        bool
	}{
		{name: "exact match", containerName: "web-abc12345", wantID: "service:web", wantOK: true},
		{name: "different service that happens to prefix-match", containerName: "web-worker-abc12345", wantID: "service:web-worker", wantOK: true},
		{name: "unrelated name", containerName: "totally-unrelated", wantOK: false},
		{name: "right prefix, wrong suffix length", containerName: "web-abc", wantOK: false},
		{name: "right prefix, non-hex suffix", containerName: "web-zzzzzzzz", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := resolveResourceID(services, tt.containerName)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestEvaluateCrashloop_BelowThreshold_NotFiring(t *testing.T) {
	tr := NewRestartTracker()
	now := time.Now()
	tr.Observe("service:web", "web-h1", now)
	tr.Observe("service:web", "web-h1", now.Add(time.Second)) // 1 restart

	r := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 3, RestartWindow: time.Hour}
	got := EvaluateCrashloop(tr, r, now.Add(time.Minute))

	if got.Firing {
		t.Error("Firing = true, want false: only 1 restart against a threshold of 3")
	}
	if got.LastValue == nil || *got.LastValue != 1 {
		t.Errorf("LastValue = %v, want 1", got.LastValue)
	}
}

func TestEvaluateCrashloop_AtThreshold_FiresImmediately(t *testing.T) {
	tr := NewRestartTracker()
	now := time.Now()
	tr.Observe("service:web", "web-h1", now)
	tr.Observe("service:web", "web-h1", now.Add(1*time.Second))
	tr.Observe("service:web", "web-h1", now.Add(2*time.Second))
	tr.Observe("service:web", "web-h1", now.Add(3*time.Second)) // 3 restarts

	r := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 3, RestartWindow: time.Hour}
	got := EvaluateCrashloop(tr, r, now.Add(time.Minute))

	if !got.Firing {
		t.Error("Firing = false, want true: 3 restarts meets a threshold of 3, no ForDuration debounce for crashloop")
	}
	if got.FiringSince == nil {
		t.Error("FiringSince = nil, want set")
	}
}

func TestEvaluateCrashloop_RestartsAgeOutOfWindow_Clears(t *testing.T) {
	tr := NewRestartTracker()
	now := time.Now()
	tr.Observe("service:web", "web-h1", now)
	tr.Observe("service:web", "web-h1", now.Add(time.Second))
	tr.Observe("service:web", "web-h1", now.Add(2*time.Second))
	tr.Observe("service:web", "web-h1", now.Add(3*time.Second))

	r := Rule{ID: "cl1", Kind: KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 3, RestartWindow: time.Minute, Firing: true}
	// Evaluate far enough later that the restart window (1m) no longer covers those old restarts.
	got := EvaluateCrashloop(tr, r, now.Add(time.Hour))

	if got.Firing {
		t.Error("Firing = true, want false: all restarts have aged out of the 1m window")
	}
}

// fakeEventSource and fakeServiceLister support Run's integration test.
type fakeEventSource struct {
	events chan docker.Event
	errs   chan error
}

func (f *fakeEventSource) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return f.events, f.errs
}

type fakeServiceLister struct {
	services []store.DesiredService
	err      error
}

func (f *fakeServiceLister) ListDesiredServices(_ context.Context) ([]store.DesiredService, error) {
	return f.services, f.err
}

func TestRestartTracker_Run_ResolvesAndTracksRealEvents(t *testing.T) {
	tr := NewRestartTracker()
	source := &fakeEventSource{events: make(chan docker.Event, 10), errs: make(chan error, 1)}
	lister := &fakeServiceLister{services: []store.DesiredService{{Name: "web"}}}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- tr.Run(ctx, source, lister, 20*time.Millisecond, nil) }()

	now := time.Now()
	source.events <- docker.Event{Action: docker.EventStart, ContainerName: "web-abc12345", Time: now}
	source.events <- docker.Event{Action: docker.EventStart, ContainerName: "web-abc12345", Time: now.Add(time.Second)} // restart
	source.events <- docker.Event{Action: docker.EventDie, ContainerName: "web-abc12345", Time: now.Add(2 * time.Second)}
	source.events <- docker.Event{Action: docker.EventStart, ContainerName: "unrelated-thing-99999999", Time: now} // no matching service: ignored

	<-ctx.Done()
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}

	if got := tr.CountSince("service:web", now.Add(-time.Hour)); got != 1 {
		t.Errorf("CountSince() = %d, want 1 (one real restart; the die event and the unrelated container's start must not count)", got)
	}
}
