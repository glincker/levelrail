package application

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/deployurls"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// tickState keeps the sleeping mark and the suspended flag apart, like the real store.
type tickState struct {
	svc   *store.DesiredService
	row   store.AppSleep
	woke  int
	clock time.Time
}

func (s *tickState) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	cp := *s.svc
	return &cp, nil
}
func (s *tickState) UpdateServiceSuspended(_ context.Context, _ string, v bool) error {
	s.svc.Suspended = v
	return nil
}
func (s *tickState) DeployURLApp(context.Context, string) (string, bool, error) {
	return "web", true, nil
}
func (s *tickState) ListAppSleep(context.Context) ([]store.AppSleep, error) {
	return []store.AppSleep{s.row}, nil
}
func (s *tickState) GetAppSleep(context.Context, string) (store.AppSleep, error) { return s.row, nil }
func (s *tickState) SetAppSleeping(_ context.Context, _ string, v bool, now time.Time) error {
	s.row.Sleeping, s.row.Since = v, now
	return nil
}
func (s *tickState) AddSleepEvent(_ context.Context, e store.SleepEvent) (int64, error) {
	if e.Kind == store.SleepEventWoke {
		s.woke++
	}
	return 1, nil
}
func (s *tickState) AddAppEvent(context.Context, store.AppEvent) error { return nil }

type noTraffic struct{}

func (noTraffic) LatestByMetric(context.Context, string) ([]telemetry.Sample, error) {
	return nil, nil
}

type tickRig struct {
	st    *tickState
	sched *appsleep.Scheduler
	ctrl  *Controller
	rt    *fakeRuntime
}

func newTickRig(t *testing.T, suspended, sleeping bool) *tickRig {
	t.Helper()
	srv := alwaysHealthy()
	t.Cleanup(srv.Close)
	rt := newFakeRuntime(serverPort(t, srv))
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	st := &tickState{
		svc:   &store.DesiredService{Name: "web-aaaaaaa", Image: "img:v1", Port: 80, Replicas: 1, Suspended: suspended},
		row:   store.AppSleep{ServiceName: "web-aaaaaaa", IdleMinutes: 30, Sleeping: sleeping, Since: now.Add(-time.Hour)},
		clock: now,
	}
	sched := &appsleep.Scheduler{
		Sleep: st, Apps: st, Traffic: noTraffic{}, Events: st,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return st.clock }, ObservedFrom: now.Add(-2 * time.Hour),
	}
	g := &deployurls.Guard{Store: st, Max: 3}
	c := New("web-aaaaaaa", st, rt, WithRetainedDeploys(g), WithReadyBudget(1e9))
	return &tickRig{st: st, sched: sched, ctrl: c, rt: rt}
}

func (r *tickRig) tick(t *testing.T) {
	t.Helper()
	if err := r.sched.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// (a) a project start unsuspends a sleeping copy: Tick re-suspends it, nothing boots.
func TestTickResuspendsSleepingCopyAfterProjectStart(t *testing.T) {
	r := newTickRig(t, false, true)
	r.tick(t)
	passes(t, r.ctrl, 3)
	if len(r.rt.containers) != 0 {
		t.Fatalf("created %d containers for a sleeping copy", len(r.rt.containers))
	}
	if !r.st.row.Sleeping || !r.st.svc.Suspended || r.st.woke != 0 {
		t.Fatalf("sleeping=%v suspended=%v woke=%d", r.st.row.Sleeping, r.st.svc.Suspended, r.st.woke)
	}
}

// (b) a real wake brings the copy up and a later Tick leaves it running.
func TestTickKeepsLegitimatelyWokenCopy(t *testing.T) {
	r := newTickRig(t, true, true)
	woke, err := appsleep.Wake(context.Background(), r.st, r.st, nil, "web-aaaaaaa", r.st.clock)
	if err != nil || !woke {
		t.Fatalf("woke=%v err=%v", woke, err)
	}
	r.st.clock = r.st.clock.Add(time.Minute)
	r.tick(t)
	passes(t, r.ctrl, 3)
	if r.st.row.Sleeping || r.st.svc.Suspended || len(r.rt.containers) == 0 {
		t.Fatalf("sleeping=%v suspended=%v containers=%d", r.st.row.Sleeping, r.st.svc.Suspended, len(r.rt.containers))
	}
}

// (c) a Wake that died after clearing the mark is repaired, and a later Wake works.
func TestTickRepairsCrashedWake(t *testing.T) {
	r := newTickRig(t, true, false)
	r.tick(t)
	if !r.st.row.Sleeping || !r.st.svc.Suspended {
		t.Fatalf("not repaired: sleeping=%v suspended=%v", r.st.row.Sleeping, r.st.svc.Suspended)
	}
	woke, err := appsleep.Wake(context.Background(), r.st, r.st, nil, "web-aaaaaaa", r.st.clock)
	if err != nil || !woke || r.st.svc.Suspended || r.st.row.Sleeping {
		t.Fatalf("woke=%v err=%v", woke, err)
	}
}

func (s *tickState) ListAppStreamsForService(context.Context, string) ([]store.AppStream, error) {
	return nil, nil
}
