package upgradesched

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

type fakeHost struct {
	mu        sync.Mutex
	latest    Candidate
	latestErr error
	plan      Plan
	planErr   error
	preflight []PreflightResult
	preCalls  int
	backups   int
	backupErr error
	applied   []string
	acked     [][]string
	applyErr  error
	running   bool
	attempt   *AttemptInfo
}

func (h *fakeHost) Latest(context.Context) (Candidate, bool, error) {
	return h.latest, h.latest.Tag != "", h.latestErr
}

func (h *fakeHost) Plan(_ context.Context, target string) (Plan, error) {
	if h.planErr != nil {
		return Plan{}, h.planErr
	}
	p := h.plan
	p.Target = target
	return p, nil
}

func (h *fakeHost) Preflight(context.Context, string) (PreflightResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	i := h.preCalls
	if i >= len(h.preflight) {
		i = len(h.preflight) - 1
	}
	h.preCalls++
	return h.preflight[i], nil
}

func (h *fakeHost) BackupNow(context.Context) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.backups++
	return "backup.db", h.backupErr
}

func (h *fakeHost) Apply(_ context.Context, target string, ack []string, initiator string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.applied = append(h.applied, target+"@"+initiator)
	h.acked = append(h.acked, ack)
	return h.applyErr
}

func (h *fakeHost) Running(context.Context) bool { return h.running }

func (h *fakeHost) Attempt(context.Context, string) (AttemptInfo, bool, error) {
	if h.attempt == nil {
		return AttemptInfo{}, false, nil
	}
	return *h.attempt, true, nil
}

type notices struct {
	mu    sync.Mutex
	texts []string
}

func (n *notices) Notify(_ context.Context, _ []string, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.texts = append(n.texts, text)
	return nil
}

type harness struct {
	t       *testing.T
	db      *store.DB
	host    *fakeHost
	notes   *notices
	now     time.Time
	current string
	audits  []string
	n       int
}

var green = PreflightResult{Checks: []upgrade.Check{{Code: "disk_space", Status: upgrade.StatusOK}}}

func staleBackup() PreflightResult {
	return PreflightResult{Blocked: true, Checks: []upgrade.Check{{
		Code: "control_plane_backup", Name: "Recent control plane backup", Status: upgrade.StatusFail,
		Message: "newest backup is 30h old", Fix: &upgrade.Fix{Action: upgrade.FixBackupNow},
	}}}
}

func lowDisk() PreflightResult {
	return PreflightResult{Blocked: true, Checks: []upgrade.Check{{
		Code: "disk_space", Name: "Free disk space", Status: upgrade.StatusFail, Message: "100 MiB free",
		Fix: &upgrade.Fix{Action: upgrade.FixRecheck},
	}}}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &harness{
		t: t, db: db, notes: &notices{}, now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC), current: "v0.5.0",
		host: &fakeHost{
			latest:    Candidate{Tag: "v0.5.1", PublishedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
			plan:      Plan{Current: "v0.5.0", NotesAvailable: true, CanApply: true},
			preflight: []PreflightResult{green},
		},
	}
}

func (h *harness) sched() *Scheduler {
	return &Scheduler{
		Store: h.db, Host: h.host, Notifier: h.notes, Config: DefaultConfig(),
		Now:     func() time.Time { return h.now },
		Current: func() string { return h.current },
		Audit:   func(_ context.Context, d string) { h.audits = append(h.audits, d) },
		NewID:   func() string { h.n++; return "us_test" + strconv.Itoa(h.n) },
	}
}

func (h *harness) setPolicy(p store.UpgradePolicy) {
	h.t.Helper()
	p.NotifyChannelIDs = `["ch_1"]`
	if err := h.db.UpdateUpgradePolicy(context.Background(), p); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) active() *store.UpgradeSchedule {
	h.t.Helper()
	a, err := h.db.ActiveUpgradeSchedule(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return a
}

func (h *harness) latestRow() store.UpgradeSchedule {
	h.t.Helper()
	list, err := h.db.ListUpgradeSchedules(context.Background(), 1)
	if err != nil || len(list) == 0 {
		h.t.Fatalf("no schedule rows: %v", err)
	}
	return list[0]
}

func (h *harness) tick(s *Scheduler) {
	h.t.Helper()
	if err := s.Tick(context.Background()); err != nil {
		h.t.Fatalf("tick: %v", err)
	}
}

func (h *harness) schedule(s *Scheduler, at time.Time) store.UpgradeSchedule {
	h.t.Helper()
	sc, err := s.Schedule(context.Background(), Request{Target: "v0.5.1", At: at, Actor: "root"})
	if err != nil {
		h.t.Fatalf("schedule: %v", err)
	}
	return sc
}

func TestScheduledUpgradeFiresAtItsTime(t *testing.T) {
	h := newHarness(t)
	s := h.sched()
	runAt := h.now.Add(6 * time.Hour)
	sc := h.schedule(s, runAt)

	h.tick(s)
	if len(h.host.applied) != 0 || h.host.backups != 0 {
		t.Fatalf("nothing may run before the scheduled time: applied=%v backups=%d", h.host.applied, h.host.backups)
	}

	h.now = runAt.Add(time.Second)
	h.tick(s)
	if len(h.host.applied) != 1 || h.host.applied[0] != "v0.5.1@schedule:"+sc.ID {
		t.Fatalf("applied = %v", h.host.applied)
	}
	if h.host.backups != 1 {
		t.Fatalf("a fresh backup must be taken before applying, got %d", h.host.backups)
	}
	if got := h.active(); got == nil || got.State != store.UpgradeScheduleRunning || got.FiredAt == "" {
		t.Fatalf("schedule must be recorded as running: %+v", got)
	}

	h.tick(s)
	if len(h.host.applied) != 1 {
		t.Fatalf("a second tick must not apply again: %v", h.host.applied)
	}
}

func TestRestartAfterFireNeverAppliesTwiceAndRecordsOutcome(t *testing.T) {
	h := newHarness(t)
	runAt := h.now.Add(time.Hour)
	h.schedule(h.sched(), runAt)
	h.now = runAt
	h.tick(h.sched())
	if len(h.host.applied) != 1 {
		t.Fatalf("applied = %v", h.host.applied)
	}

	// The control plane was replaced: a brand new scheduler over the same
	// database, now running the target, with the attempt finished.
	h.current = "v0.5.1"
	h.host.attempt = &AttemptInfo{ID: "att-1", Outcome: "succeeded"}
	h.now = h.now.Add(2 * time.Minute)
	restarted := h.sched()
	h.tick(restarted)

	if len(h.host.applied) != 1 {
		t.Fatalf("restart re-applied: %v", h.host.applied)
	}
	if got := h.latestRow(); got.State != store.UpgradeScheduleApplied {
		t.Fatalf("state = %s, want applied", got.State)
	}
	if len(h.notes.texts) == 0 || !strings.Contains(h.notes.texts[len(h.notes.texts)-1], "upgraded to v0.5.1") {
		t.Fatalf("a success notice is expected, got %v", h.notes.texts)
	}
}

func TestRestartWhilePendingStillFires(t *testing.T) {
	h := newHarness(t)
	runAt := h.now.Add(time.Hour)
	h.schedule(h.sched(), runAt)

	h.now = runAt.Add(10 * time.Minute)
	h.tick(h.sched())
	if len(h.host.applied) != 1 {
		t.Fatalf("a fresh process must pick up a persisted schedule and fire it: %v", h.host.applied)
	}
}

func TestFireTimePreflightFailureDoesNotApplyAndRetries(t *testing.T) {
	h := newHarness(t)
	h.host.preflight = []PreflightResult{lowDisk(), lowDisk(), green}
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)

	h.now = runAt
	h.tick(s)
	if len(h.host.applied) != 0 || h.host.backups != 0 {
		t.Fatalf("a blocked preflight must not back up or apply: applied=%v backups=%d", h.host.applied, h.host.backups)
	}
	got := h.active()
	if got == nil || got.State != store.UpgradeScheduleBlocked || !strings.Contains(got.LastReason, "Free disk space") || got.CheckCount != 1 {
		t.Fatalf("blocked state not recorded: %+v", got)
	}
	if len(h.notes.texts) != 1 {
		t.Fatalf("one alert expected on the transition, got %v", h.notes.texts)
	}

	h.now = h.now.Add(30 * time.Second)
	h.tick(s)
	if h.host.preCalls != 1 {
		t.Fatalf("retry must respect the retry interval, preflight ran %d times", h.host.preCalls)
	}

	h.now = h.now.Add(3 * time.Minute)
	h.tick(s)
	if len(h.notes.texts) != 1 {
		t.Fatalf("the same reason must not alert again: %v", h.notes.texts)
	}
	h.now = h.now.Add(3 * time.Minute)
	h.tick(s)
	if len(h.host.applied) != 1 {
		t.Fatalf("once the blocker clears inside the grace period it must apply: %v", h.host.applied)
	}
}

func TestStaleBackupIsFixedByTheScheduler(t *testing.T) {
	h := newHarness(t)
	h.host.preflight = []PreflightResult{staleBackup(), green}
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.now = runAt
	h.tick(s)

	if h.host.backups != 1 {
		t.Fatalf("exactly one backup expected (fix, not fix plus pre-apply), got %d", h.host.backups)
	}
	if len(h.host.applied) != 1 {
		t.Fatalf("applied = %v", h.host.applied)
	}
}

func TestBackupFailureBlocksTheUpgrade(t *testing.T) {
	h := newHarness(t)
	h.host.backupErr = errors.New("disk full")
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.now = runAt
	h.tick(s)
	if len(h.host.applied) != 0 {
		t.Fatalf("no apply without a backup: %v", h.host.applied)
	}
	if got := h.active(); got == nil || got.State != store.UpgradeScheduleBlocked || !strings.Contains(got.LastReason, "backup failed") {
		t.Fatalf("got %+v", got)
	}
}

func TestMissedWhenTheGraceEndsBlocked(t *testing.T) {
	h := newHarness(t)
	h.host.preflight = []PreflightResult{lowDisk()}
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.now = runAt
	h.tick(s)
	h.now = runAt.Add(3 * time.Hour)
	h.tick(s)

	if h.active() != nil {
		t.Fatal("a missed schedule must stop being active")
	}
	if got := h.latestRow(); got.State != store.UpgradeScheduleMissed || !strings.Contains(got.LastReason, "Free disk space") {
		t.Fatalf("got %+v", got)
	}
	if len(h.host.applied) != 0 {
		t.Fatal("must not apply after the window")
	}
}

func TestTargetMustStillBeOnTheChannel(t *testing.T) {
	h := newHarness(t)
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.host.planErr = ErrReleaseNotFound
	h.now = runAt
	h.tick(s)
	if got := h.latestRow(); got.State != store.UpgradeScheduleFailed || len(h.host.applied) != 0 {
		t.Fatalf("got %+v applied=%v", got, h.host.applied)
	}
}

func TestNewBreakingChangeAfterSchedulingBlocksApply(t *testing.T) {
	h := newHarness(t)
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.host.plan.Breaking = []Breaking{{ID: "b-1", RequiresAck: true}}
	h.now = runAt
	h.tick(s)
	if got := h.latestRow(); got.State != store.UpgradeScheduleFailed || !strings.Contains(got.LastReason, "b-1") || len(h.host.applied) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestApplyLaunchFailureIsRecorded(t *testing.T) {
	h := newHarness(t)
	h.host.applyErr = errors.New("systemd-run missing")
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.now = runAt
	h.tick(s)
	if got := h.latestRow(); got.State != store.UpgradeScheduleFailed || !strings.Contains(got.LastReason, "systemd-run missing") {
		t.Fatalf("got %+v", got)
	}
}

func TestRolledBackAttemptMarksScheduleFailed(t *testing.T) {
	h := newHarness(t)
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.now = runAt
	h.tick(s)
	h.host.attempt = &AttemptInfo{ID: "att", Outcome: "rolled_back", Error: "health check failed"}
	h.tick(s)
	got := h.latestRow()
	if got.State != store.UpgradeScheduleFailed || !strings.Contains(got.LastReason, "health check failed") {
		t.Fatalf("got %+v", got)
	}
}

func TestRunningWithNoAttemptTimesOut(t *testing.T) {
	h := newHarness(t)
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	h.now = runAt
	h.tick(s)
	h.now = h.now.Add(5 * time.Minute)
	h.tick(s)
	if h.active() == nil {
		t.Fatal("still inside the start timeout")
	}
	h.now = h.now.Add(10 * time.Minute)
	h.tick(s)
	if got := h.latestRow(); got.State != store.UpgradeScheduleFailed {
		t.Fatalf("got %+v", got)
	}
}

func TestCancelWinsBeforeFireAndLosesAfter(t *testing.T) {
	h := newHarness(t)
	s := h.sched()
	runAt := h.now.Add(time.Hour)
	h.schedule(s, runAt)
	if _, err := s.Cancel(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	h.now = runAt
	h.tick(s)
	if len(h.host.applied) != 0 {
		t.Fatal("a cancelled schedule must not fire")
	}
	if _, err := s.Cancel(context.Background(), "root"); !errors.Is(err, ErrNothingScheduled) {
		t.Fatalf("err = %v", err)
	}

	h.schedule(s, h.now.Add(time.Hour))
	h.now = h.now.Add(time.Hour)
	h.tick(s)
	if _, err := s.Cancel(context.Background(), "root"); !errors.Is(err, ErrRunning) {
		t.Fatalf("a running upgrade cannot be cancelled, err = %v", err)
	}
}

func TestScheduleValidation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*Request)
		host   func(*fakeHost)
		want   error
	}{
		{"past time", func(r *Request) { r.At = h.now.Add(-time.Hour) }, nil, ErrInvalid},
		{"too far ahead", func(r *Request) { r.At = h.now.Add(200 * 24 * time.Hour) }, nil, ErrInvalid},
		{"no time at all", func(r *Request) { r.At = time.Time{} }, nil, ErrInvalid},
		{"not newer", func(r *Request) { r.Target = "v0.4.0" }, nil, ErrInvalid},
		{"unknown release", nil, func(h *fakeHost) { h.planErr = ErrReleaseNotFound }, ErrReleaseNotFound},
		{"breaking needs ack", nil, func(h *fakeHost) { h.plan.Breaking = []Breaking{{ID: "b", RequiresAck: true}} }, ErrNeedsAck},
		{"major jump needs ack", func(r *Request) { r.Target = "v1.0.0" }, nil, ErrNeedsMajorAck},
		{"notes unavailable", nil, func(h *fakeHost) { h.plan.NotesAvailable = false }, ErrInvalid},
		{"host cannot apply", nil, func(h *fakeHost) { h.plan.CanApply = false }, ErrInvalid},
		{"no maintenance window", func(r *Request) { r.Window = true }, nil, ErrNoWindow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			s := h.sched()
			req := Request{Target: "v0.5.1", At: h.now.Add(2 * time.Hour), Actor: "root"}
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			if tc.host != nil {
				tc.host(h.host)
			}
			if _, err := s.Schedule(ctx, req); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if h.active() != nil {
				t.Fatal("a refused request must not create a schedule")
			}
		})
	}
	_ = h
}

func TestScheduleAckedBreakingAndMajor(t *testing.T) {
	h := newHarness(t)
	h.host.plan.Breaking = []Breaking{{ID: "b", RequiresAck: true}}
	s := h.sched()
	sc, err := s.Schedule(context.Background(), Request{Target: "v1.0.0", At: h.now.Add(time.Hour), Ack: []string{"b"}, MajorAck: true, Actor: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if !sc.MajorAcked || !strings.Contains(sc.AckedJSON, `"b"`) {
		t.Fatalf("acks not persisted: %+v", sc)
	}
	if _, err := s.Schedule(context.Background(), Request{At: h.now.Add(2 * time.Hour), Ack: []string{"b"}, Actor: "root"}); !errors.Is(err, ErrAlreadyScheduled) {
		t.Fatalf("second schedule err = %v", err)
	}
	if _, err := s.Schedule(context.Background(), Request{At: h.now.Add(2 * time.Hour), Ack: []string{"b"}, Replace: true, Actor: "root"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
}

func TestScheduleNextWindowUsesPolicyWindow(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(store.UpgradePolicy{Mode: "off", WindowCron: "0 3 * * *", WindowDurationSeconds: 7200, WindowTimezone: "Europe/Berlin", MinReleaseAgeDays: 3})
	s := h.sched()
	sc, err := s.Schedule(context.Background(), Request{Target: "v0.5.1", Window: true, Actor: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if sc.RunAt != "2026-10-12T01:00:00Z" || sc.RetryUntil != "2026-10-12T03:00:00Z" || sc.Mode != store.UpgradeModeWindow {
		t.Fatalf("window not resolved in the policy timezone: %+v", sc)
	}
}

func autoPolicy(mode string) store.UpgradePolicy {
	return store.UpgradePolicy{Mode: mode, WindowCron: "0 3 * * *", WindowDurationSeconds: 7200, WindowTimezone: "UTC", MinReleaseAgeDays: 3}
}

func TestAutoUpgradePolicyGating(t *testing.T) {
	tests := []struct {
		name         string
		policy       store.UpgradePolicy
		host         func(*fakeHost)
		wantSchedule bool
		wantDecision string
		wantNotice   bool
	}{
		{"off creates nothing", autoPolicy("off"), nil, false, "", false},
		{"eligible patch is scheduled in the window", autoPolicy("patch"), nil, true, "scheduled", true},
		{"paused", func() store.UpgradePolicy { p := autoPolicy("any"); p.Paused = true; return p }(), nil, false, "paused", false},
		{"unacked breaking change is never auto-applied", autoPolicy("any"),
			func(h *fakeHost) { h.plan.Breaking = []Breaking{{ID: "b", RequiresAck: true}} }, false, "breaking change", true},
		{"too young waits", autoPolicy("any"),
			func(h *fakeHost) { h.latest.PublishedAt = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC) }, false, "minimum release age", false},
		{"minor release is held in patch mode", autoPolicy("patch"),
			func(h *fakeHost) { h.latest.Tag = "v0.6.0" }, false, "not a patch release", false},
		{"major jump is held", autoPolicy("any"),
			func(h *fakeHost) { h.latest.Tag = "v1.0.0" }, false, "major version", true},
		{"informational breaking note does not hold", autoPolicy("any"),
			func(h *fakeHost) { h.plan.Breaking = []Breaking{{ID: "i", RequiresAck: false}} }, true, "scheduled", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.setPolicy(tc.policy)
			if tc.host != nil {
				tc.host(h.host)
			}
			s := h.sched()
			h.tick(s)
			got := h.active()
			if (got != nil) != tc.wantSchedule {
				t.Fatalf("schedule created = %v, want %v", got != nil, tc.wantSchedule)
			}
			row, _ := h.db.GetUpgradePolicy(context.Background())
			if !strings.Contains(row.Decision, tc.wantDecision) {
				t.Fatalf("decision = %q, want it to mention %q", row.Decision, tc.wantDecision)
			}
			if (len(h.notes.texts) > 0) != tc.wantNotice {
				t.Fatalf("notices = %v, want notice=%v", h.notes.texts, tc.wantNotice)
			}
			if got != nil {
				if got.Source != store.UpgradeSourceAuto || got.RunAt != "2026-10-12T03:00:00Z" || got.RetryUntil != "2026-10-12T05:00:00Z" {
					t.Fatalf("automatic schedule not placed in the window: %+v", got)
				}
			}
			h.tick(s)
			if len(h.notes.texts) > 1 {
				t.Fatalf("a repeated verdict must not re-notify: %v", h.notes.texts)
			}
		})
	}
}

func TestAutoUpgradeFiresInsideWindowAndIsCancelledWhenPaused(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(autoPolicy("any"))
	s := h.sched()
	h.tick(s)
	sc := h.active()
	if sc == nil {
		t.Fatal("expected an automatic schedule")
	}

	h.now = time.Date(2026, 10, 12, 3, 0, 5, 0, time.UTC)
	h.tick(s)
	if len(h.host.applied) != 1 || h.host.applied[0] != "v0.5.1@auto:"+sc.ID {
		t.Fatalf("applied = %v", h.host.applied)
	}

	h2 := newHarness(t)
	h2.setPolicy(autoPolicy("any"))
	s2 := h2.sched()
	h2.tick(s2)
	p := autoPolicy("any")
	p.Paused = true
	h2.setPolicy(p)
	h2.now = time.Date(2026, 10, 12, 3, 0, 5, 0, time.UTC)
	h2.tick(s2)
	if len(h2.host.applied) != 0 {
		t.Fatalf("pausing must stop an automatic schedule: %v", h2.host.applied)
	}
	if got := h2.latestRow(); got.State != store.UpgradeScheduleCancelled {
		t.Fatalf("state = %s", got.State)
	}
}

func TestAutoDoesNotRetryAReleaseThatFailedBefore(t *testing.T) {
	h := newHarness(t)
	h.setPolicy(autoPolicy("any"))
	s := h.sched()
	h.tick(s)
	h.now = time.Date(2026, 10, 12, 3, 0, 5, 0, time.UTC)
	h.tick(s)
	h.host.attempt = &AttemptInfo{Outcome: "rolled_back", Error: "unhealthy"}
	h.tick(s)
	if got := h.latestRow(); got.State != store.UpgradeScheduleFailed {
		t.Fatalf("state = %s", got.State)
	}
	h.host.attempt = nil
	h.now = h.now.Add(24 * time.Hour)
	h.tick(s)
	if h.active() != nil {
		t.Fatal("a release that already failed automatically must not be retried automatically")
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{"APP_UPGRADE_SCHEDULE_GRACE": "30m", "APP_UPGRADE_RETRY_INTERVAL": "junk", "APP_UPGRADE_START_TIMEOUT": "-1m"}
	c := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	d := DefaultConfig()
	if c.Grace != 30*time.Minute || c.RetryInterval != d.RetryInterval || c.StartTimeout != d.StartTimeout {
		t.Fatalf("config = %+v", c)
	}
}
