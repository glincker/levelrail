package deploywatch

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeInspector map[string]*docker.ExitState

func (f fakeInspector) InspectExitState(_ context.Context, name string) (*docker.ExitState, error) {
	return f[name], nil
}

type fakeRestarts int

func (f fakeRestarts) CountSince(string, time.Time) int { return int(f) }

type fakeMetrics map[string]float64

func (f fakeMetrics) QueryMetrics(_ context.Context, _ string, metric string, _, to time.Time) ([]telemetry.Sample, error) {
	return []telemetry.Sample{{Metric: metric, Value: f[metric], Timestamp: to}}, nil
}

type notifications struct {
	mu   sync.Mutex
	msgs []string
}

func (n *notifications) notify(_ context.Context, _, _, message string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.msgs = append(n.msgs, message)
}

type watchFixture struct {
	db      *store.DB
	watcher *Watcher
	notes   *notifications
	now     time.Time
}

func newWatchFixture(t *testing.T, withPin bool) *watchFixture {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now()

	good := store.DesiredService{Name: "web", Image: "app:v1", Port: 8080, Env: map[string]string{"MODE": "old"}}
	saveAttempt(t, db, "dep_v1", good, now.Add(-time.Hour))
	bad := good
	bad.Image, bad.Env = "app:v2", map[string]string{"MODE": "new"}
	if err := db.SaveDesiredService(ctx, bad); err != nil {
		t.Fatal(err)
	}
	saveAttempt(t, db, "dep_v2", bad, now.Add(-2*time.Minute))
	if withPin {
		if err := db.SaveRollbackPin(ctx, store.RollbackPin{ServiceName: "web", DeployAttemptID: "dep_v1", Image: "app:v1", ImageID: "sha256:v1", PinRef: "x-rollback/web:dep_v1", CreatedAt: now.Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordGateEvent(ctx, store.DeployGateEvent{DeployAttemptID: "dep_v2", ServiceName: "web", Phase: deploysafety.PhaseCutover, Status: deploysafety.StatusPassed, At: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}

	notes := &notifications{}
	defaults := deploysafety.BuiltinDefaults()
	defaults.WatchMode, defaults.AutoRollback = deploysafety.WatchModeAuto, true
	w := &Watcher{Store: db, Defaults: defaults, Notify: notes.notify, now: func() time.Time { return now }}
	return &watchFixture{db: db, watcher: w, notes: notes, now: now}
}

func saveAttempt(t *testing.T, db *store.DB, id string, svc store.DesiredService, started time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: svc.Name, Image: svc.Image, Status: store.DeployAttemptStatusRunning, StartedAt: started, Snapshot: store.NewDeployAttemptSnapshot(svc)}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, started.Add(time.Second), ""); err != nil {
		t.Fatal(err)
	}
}

func (f *watchFixture) events(t *testing.T) []store.DeployGateEvent {
	t.Helper()
	events, err := f.db.ListGateEvents(context.Background(), "dep_v2")
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func hasEvent(events []store.DeployGateEvent, phase, status, reason string) bool {
	for _, e := range events {
		if e.Phase == phase && e.Status == status && (reason == "" || e.Reason == reason) {
			return true
		}
	}
	return false
}

func currentContainer() string {
	svc := store.DesiredService{Name: "web", Image: "app:v2"}
	return application.ReplicaContainerName("web", application.NameImage(svc), "", 0)
}

func TestWatcher_TripsAndRollsBackWithConfig(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(w *Watcher)
		wantReason string
	}{
		{"crashloop", func(w *Watcher) { w.Restarts = fakeRestarts(2) }, deploysafety.ReasonWatchCrashloop},
		{"oom kill", func(w *Watcher) {
			w.Inspectors = func(string) (docker.ExitStateInspector, error) {
				return fakeInspector{currentContainer(): {Running: false, OOMKilled: true, ExitCode: 137}}, nil
			}
		}, deploysafety.ReasonWatchOOMKilled},
		{"5xx spike", func(w *Watcher) {
			w.Metrics = fakeMetrics{telemetry.MetricHTTPRequests: 100, telemetry.MetricHTTPResponses5xx: 60}
		}, deploysafety.ReasonWatchErrorRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newWatchFixture(t, true)
			tt.setup(f.watcher)
			ctx := context.Background()
			f.watcher.Tick(ctx)

			events := f.events(t)
			if !hasEvent(events, deploysafety.PhaseWatch, deploysafety.StatusFailed, tt.wantReason) {
				t.Fatalf("events = %+v, want a failed watch with %s", events, tt.wantReason)
			}
			if !hasEvent(events, deploysafety.PhaseRollback, deploysafety.StatusPassed, deploysafety.ReasonAutoRolledBack) {
				t.Errorf("events = %+v, want a passed rollback", events)
			}
			bad, _ := f.db.GetDeployAttempt(ctx, "dep_v2")
			if bad.Status != store.DeployAttemptStatusFailed || bad.Reason != tt.wantReason {
				t.Errorf("tripped attempt = status %q reason %q, want failed/%s", bad.Status, bad.Reason, tt.wantReason)
			}
			svc, _ := f.db.GetDesiredService(ctx, "web")
			if svc.Image != "x-rollback/web:dep_v1" || svc.Env["MODE"] != "old" {
				t.Errorf("desired after rollback = image %q env %v, want the pinned v1 image and its config", svc.Image, svc.Env)
			}
			attempts, _ := f.db.ListDeployAttempts(ctx, "web")
			if attempts[0].Source != store.DeployAttemptSourceAutoRollback || attempts[0].Reason != deploysafety.ReasonAutoRolledBack {
				t.Errorf("newest attempt = %+v, want an auto_rollback attempt", attempts[0])
			}
			if len(f.notes.msgs) != 1 || !strings.Contains(f.notes.msgs[0], "rolled back automatically") {
				t.Errorf("notifications = %v", f.notes.msgs)
			}

			f.watcher.Tick(ctx)
			if again, _ := f.db.ListDeployAttempts(ctx, "web"); len(again) != len(attempts) {
				t.Errorf("second tick deployed again: %d attempts, want %d", len(again), len(attempts))
			}
		})
	}
}

func TestWatcher_QuietReleaseClearsWindow(t *testing.T) {
	f := newWatchFixture(t, true)
	ctx := context.Background()
	f.watcher.Tick(ctx)
	if events := f.events(t); !hasEvent(events, deploysafety.PhaseWatch, deploysafety.StatusStarted, "") || hasEvent(events, deploysafety.PhaseWatch, deploysafety.StatusPassed, "") {
		t.Fatalf("events inside window = %+v, want started only", events)
	}
	f.watcher.now = func() time.Time { return f.now.Add(10 * time.Minute) }
	f.watcher.Tick(ctx)
	if events := f.events(t); !hasEvent(events, deploysafety.PhaseWatch, deploysafety.StatusPassed, "") {
		t.Errorf("events after window = %+v, want watch passed", events)
	}
	if svc, _ := f.db.GetDesiredService(ctx, "web"); svc.Image != "app:v2" {
		t.Errorf("image = %q, a quiet release must stay", svc.Image)
	}
}

func TestWatcher_AutoRollbackOffOnlyAlerts(t *testing.T) {
	f := newWatchFixture(t, true)
	ctx := context.Background()
	off := false
	if err := f.db.SaveDeploySafetySettings(ctx, store.DeploySafetySettings{ServiceName: "web", AutoRollback: &off}); err != nil {
		t.Fatal(err)
	}
	f.watcher.Restarts = fakeRestarts(5)
	f.watcher.Tick(ctx)
	if svc, _ := f.db.GetDesiredService(ctx, "web"); svc.Image != "app:v2" {
		t.Errorf("image = %q, want no rollback with auto_rollback off", svc.Image)
	}
	if len(f.notes.msgs) != 1 || !strings.Contains(f.notes.msgs[0], "automatic rollback is off") {
		t.Errorf("notifications = %v", f.notes.msgs)
	}
}

type fakeLocalImages map[string]string

func (f fakeLocalImages) ResolveImage(context.Context, string, *docker.RegistryAuth, bool) (docker.ResolvedImage, error) {
	panic("a verified rollback must not go back to the registry")
}

func (f fakeLocalImages) InspectImageID(_ context.Context, ref string) (string, error) {
	return f[ref], nil
}

func TestWatcher_FallsBackToHistoryWithoutPin(t *testing.T) {
	tests := []struct {
		name      string
		local     fakeLocalImages
		wantImage string
	}{
		{"tag still holds the recorded content", fakeLocalImages{"app:v1": "sha256:v1"}, "app:v1"},
		{"tag moved since", fakeLocalImages{"app:v1": "sha256:other"}, "app:v2"},
		{"no way to check the tag", nil, "app:v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newWatchFixture(t, false)
			ctx := context.Background()
			if _, err := f.db.ExecContext(ctx, `UPDATE deploy_attempts SET image_digest = 'sha256:v1' WHERE id = 'dep_v1'`); err != nil {
				t.Fatal(err)
			}
			if tt.local != nil {
				f.watcher.Resolver = tt.local
			}
			f.watcher.Restarts = fakeRestarts(3)
			f.watcher.Tick(ctx)
			if svc, _ := f.db.GetDesiredService(ctx, "web"); svc.Image != tt.wantImage {
				t.Errorf("image = %q, want %q", svc.Image, tt.wantImage)
			}
			rolledBack := hasEvent(f.events(t), deploysafety.PhaseRollback, deploysafety.StatusPassed, "")
			if rolledBack != (tt.wantImage == "app:v1") {
				t.Errorf("rollback passed = %v, want %v", rolledBack, !rolledBack)
			}
		})
	}
}

func TestWatcher_NeverChainsRollbacks(t *testing.T) {
	f := newWatchFixture(t, true)
	ctx := context.Background()
	if _, err := f.db.ExecContext(ctx, `UPDATE deploy_attempts SET source = ? WHERE id = 'dep_v2'`, store.DeployAttemptSourceAutoRollback); err != nil {
		t.Fatal(err)
	}
	f.watcher.Restarts = fakeRestarts(3)
	f.watcher.Tick(ctx)
	if svc, _ := f.db.GetDesiredService(ctx, "web"); svc.Image != "app:v2" {
		t.Errorf("image = %q, a tripped rollback must not roll back again", svc.Image)
	}
	if !hasEvent(f.events(t), deploysafety.PhaseWatch, deploysafety.StatusFailed, deploysafety.ReasonWatchCrashloop) {
		t.Errorf("events = %+v, want the trip recorded", f.events(t))
	}
	if len(f.notes.msgs) != 1 || !strings.Contains(f.notes.msgs[0], "already an automatic rollback") {
		t.Errorf("notifications = %v", f.notes.msgs)
	}
}

func TestWatcher_SkipsFirstSampleAfterCutover(t *testing.T) {
	f := newWatchFixture(t, true)
	f.watcher.Metrics = fakeMetrics{telemetry.MetricHTTPRequests: 100, telemetry.MetricHTTPResponses5xx: 90}
	f.watcher.SampleInterval = 2 * time.Minute
	f.watcher.Tick(context.Background())
	if hasEvent(f.events(t), deploysafety.PhaseWatch, deploysafety.StatusFailed, "") {
		t.Errorf("events = %+v, the first sample after cutover can hold the previous release's answers", f.events(t))
	}
}

func TestWatcher_NoTargetAndSuperseded(t *testing.T) {
	f := newWatchFixture(t, false)
	ctx := context.Background()
	if _, err := f.db.FailDeployAttemptWithReason(ctx, "dep_v1", "x", "y"); err != nil {
		t.Fatal(err)
	}
	f.watcher.Restarts = fakeRestarts(3)
	f.watcher.Tick(ctx)
	if events := f.events(t); !hasEvent(events, deploysafety.PhaseRollback, deploysafety.StatusFailed, deploysafety.ReasonNoRollbackTarget) {
		t.Errorf("events = %+v, want NoRollbackTarget", events)
	}

	g := newWatchFixture(t, true)
	svc, _ := g.db.GetDesiredService(ctx, "web")
	svc.Image = "app:v3"
	if err := g.db.SaveDesiredService(ctx, *svc); err != nil {
		t.Fatal(err)
	}
	g.watcher.Restarts = fakeRestarts(9)
	g.watcher.Tick(ctx)
	if events := g.events(t); len(events) != 1 {
		t.Errorf("superseded release got watch events: %+v", events)
	}
}
