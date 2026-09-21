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

// fakeAutoRollbackStore is an in-memory AutoRollbackStore for
// MaybeAutoRollback's own tests: one service, its deploy attempt
// history, and a record of every SaveDesiredService/SaveDeployAttempt
// call so a test can assert whether a rollback was actually triggered.
type fakeAutoRollbackStore struct {
	svc           store.DesiredService
	getErr        error
	attempts      []store.DeployAttempt
	listErr       error
	savedServices []store.DesiredService
	savedAttempts []store.DeployAttempt
	finishedIDs   []string
}

func (f *fakeAutoRollbackStore) GetDesiredService(_ context.Context, _ string) (*store.DesiredService, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	svc := f.svc
	return &svc, nil
}

func (f *fakeAutoRollbackStore) ListDeployAttempts(_ context.Context, _ string) ([]store.DeployAttempt, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.attempts, nil
}

// SaveDesiredService both records the call (for assertions) and applies
// it to f.svc, mirroring a real store: GetDesiredService must reflect the
// image a prior rollback just set, or a test can't exercise the rearm
// path (a second MaybeAutoRollback call re-reading the same stale image).
func (f *fakeAutoRollbackStore) SaveDesiredService(_ context.Context, svc store.DesiredService) error {
	f.savedServices = append(f.savedServices, svc)
	f.svc = svc
	return nil
}

// SaveDeployAttempt both records the call and prepends a to f.attempts
// (ListDeployAttempts' own newest-first order), so a rollback triggered
// through TriggerImageDeploy is itself visible to a later
// PreviousKnownGoodImage scan, the same as it would be against a real
// store.
func (f *fakeAutoRollbackStore) SaveDeployAttempt(_ context.Context, a store.DeployAttempt) error {
	f.savedAttempts = append(f.savedAttempts, a)
	f.attempts = append([]store.DeployAttempt{a}, f.attempts...)
	return nil
}

// FinishDeployAttempt updates the matching attempt's status in place
// (mirroring how recordImageDeployAttempt marks its own attempt succeeded
// immediately), so a rollback's own attempt reads back as succeeded, not
// its initial running status, for anything that lists attempts afterward.
func (f *fakeAutoRollbackStore) FinishDeployAttempt(_ context.Context, id, status string, _ time.Time, _ string) error {
	f.finishedIDs = append(f.finishedIDs, id)
	for i := range f.attempts {
		if f.attempts[i].ID == id {
			f.attempts[i].Status = status
		}
	}
	return nil
}

func TestMaybeAutoRollback_Enabled_RollsBackToPreviousImage(t *testing.T) {
	st := &fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnCrashloop: true},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	nudger := &fakeAutoRollbackNudger{}

	MaybeAutoRollback(context.Background(), st, nudger, nil, "service:web", nil)

	if len(st.savedServices) != 1 || st.savedServices[0].Image != "web:v2" {
		t.Fatalf("savedServices = %+v, want one save with image web:v2", st.savedServices)
	}
	if len(st.savedAttempts) != 1 || st.savedAttempts[0].Source != store.DeployAttemptSourceAutoRollback {
		t.Fatalf("savedAttempts = %+v, want one with Source=%q", st.savedAttempts, store.DeployAttemptSourceAutoRollback)
	}
	if nudger.calls != 1 {
		t.Errorf("nudger.calls = %d, want 1", nudger.calls)
	}
}

func TestMaybeAutoRollback_Disabled_NeverFires(t *testing.T) {
	st := &fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnCrashloop: false},
		attempts: []store.DeployAttempt{
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	nudger := &fakeAutoRollbackNudger{}

	MaybeAutoRollback(context.Background(), st, nudger, nil, "service:web", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none: auto-rollback is off for this app", st.savedServices)
	}
	if nudger.calls != 0 {
		t.Errorf("nudger.calls = %d, want 0", nudger.calls)
	}
}

func TestMaybeAutoRollback_NoOlderImage_NoPanicNoRollback(t *testing.T) {
	st := &fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v1", AutoRollbackOnCrashloop: true},
		attempts: []store.DeployAttempt{
			{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	nudger := &fakeAutoRollbackNudger{}

	MaybeAutoRollback(context.Background(), st, nudger, nil, "service:web", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none: already on the oldest known image", st.savedServices)
	}
	if nudger.calls != 0 {
		t.Errorf("nudger.calls = %d, want 0", nudger.calls)
	}
}

func TestMaybeAutoRollback_NotServiceScopedResourceID_NoOp(t *testing.T) {
	st := &fakeAutoRollbackStore{svc: store.DesiredService{Name: "web", Image: "web:v1", AutoRollbackOnCrashloop: true}}

	MaybeAutoRollback(context.Background(), st, nil, nil, "node:some-node-id", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none for a non-service resourceID", st.savedServices)
	}
}

func TestAutoRollbackTracker_NilReceiver_NeverPanicsAlwaysUnarmed(t *testing.T) {
	var tr *AutoRollbackTracker
	if tr.armed("service:web") {
		t.Error("armed() on a nil tracker = true, want false")
	}
	if tr.alreadyHandled("service:web", "web:v1") {
		t.Error("alreadyHandled() on a nil tracker = true, want false")
	}
	tr.record("service:web", "web:v1") // must not panic
}

func TestAutoRollbackTracker_RecordThenAlreadyHandled(t *testing.T) {
	tr := NewAutoRollbackTracker()
	if tr.armed("service:web") {
		t.Error("armed() before any record = true, want false")
	}

	tr.record("service:web", "web:v1")

	if !tr.armed("service:web") {
		t.Error("armed() after a record = false, want true")
	}
	if !tr.alreadyHandled("service:web", "web:v1") {
		t.Error("alreadyHandled(same image) = false, want true")
	}
	if tr.alreadyHandled("service:web", "web:v2") {
		t.Error("alreadyHandled(different image) = true, want false: a changed desired image is a new incident")
	}
	if tr.alreadyHandled("service:other", "web:v1") {
		t.Error("alreadyHandled() leaked across resourceIDs")
	}
}

// TestMaybeAutoRollback_Rearm_NewDeployAfterRollback_FiresAgain reproduces
// the field bug directly at MaybeAutoRollback's own level: a first
// crashloop episode gets auto-rolled-back, then a second, genuinely
// different bad image is deployed before anything re-reads the store in
// between (mirroring the real gap: the crashloop rule stays continuously
// Firing because the first episode's restarts are still inside its
// RestartWindow, so Engine.Tick never sees a fresh becameFiring
// transition). MaybeAutoRollback must still roll back the second episode
// when called again with the same tracker, and must target an image that
// is not the one that just caused it.
func TestMaybeAutoRollback_Rearm_NewDeployAfterRollback_FiresAgain(t *testing.T) {
	st := &fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:badA", AutoRollbackOnCrashloop: true},
		attempts: []store.DeployAttempt{
			{Image: "web:badA", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:good", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	nudger := &fakeAutoRollbackNudger{}
	tracker := NewAutoRollbackTracker()

	// Episode 1: web:badA is crash-looping. Rolls back to web:good.
	MaybeAutoRollback(context.Background(), st, nudger, tracker, "service:web", nil)
	if len(st.savedServices) != 1 || st.savedServices[0].Image != "web:good" {
		t.Fatalf("after episode 1: savedServices = %+v, want one save with image web:good", st.savedServices)
	}

	// Rule stays Firing without ever resolving (the real bug's exact
	// condition), but a genuinely different bad image is deployed:
	// something outside MaybeAutoRollback changes the desired image and
	// records its own deploy attempt, the same as a real deploy would.
	st.svc.Image = "web:badB"
	st.attempts = append([]store.DeployAttempt{{Image: "web:badB", Status: store.DeployAttemptStatusSucceeded}}, st.attempts...)

	// Episode 2, same tracker, called again exactly as Engine.Tick's new
	// stillFiring branch would.
	MaybeAutoRollback(context.Background(), st, nudger, tracker, "service:web", nil)

	if len(st.savedServices) != 2 {
		t.Fatalf("after episode 2: savedServices = %+v, want two saves total (one per distinct bad deploy)", st.savedServices)
	}
	if got := st.savedServices[1].Image; got != "web:good" {
		t.Errorf("episode 2 rolled back to %q, want web:good (not web:badB, the image that just caused it)", got)
	}
	if nudger.calls != 2 {
		t.Errorf("nudger.calls = %d, want 2", nudger.calls)
	}

	// Nothing else changes: a third call against the same still-unresolved
	// image must not fire a third time.
	MaybeAutoRollback(context.Background(), st, nudger, tracker, "service:web", nil)
	if len(st.savedServices) != 2 {
		t.Errorf("after a repeat call with no image change: savedServices = %+v, want still exactly 2", st.savedServices)
	}
}

// fakeAutoRollbackNudger counts Nudge calls for MaybeAutoRollback's own
// tests.
type fakeAutoRollbackNudger struct{ calls int }

func (f *fakeAutoRollbackNudger) Nudge() { f.calls++ }
