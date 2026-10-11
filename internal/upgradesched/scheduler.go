package upgradesched

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/semver"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// Sentinel errors the API maps to status codes.
var (
	ErrReleaseNotFound  = errors.New("upgradesched: release not found on the configured channel")
	ErrBackupNotAvail   = errors.New("upgradesched: control plane backups are not configured")
	ErrAlreadyScheduled = errors.New("upgradesched: an upgrade is already scheduled")
	ErrRunning          = errors.New("upgradesched: the upgrade is already running")
	ErrNothingScheduled = errors.New("upgradesched: no upgrade is scheduled")
	ErrNoWindow         = errors.New("upgradesched: no maintenance window is configured")
	ErrNeedsAck         = errors.New("upgradesched: breaking changes need acknowledgement")
	ErrNeedsMajorAck    = errors.New("upgradesched: a major version jump needs acknowledgement")
	ErrInvalid          = errors.New("upgradesched: invalid request")
)

// Breaking is one breaking change between the running version and a target.
type Breaking struct {
	ID          string
	Version     string
	Summary     string
	RequiresAck bool
}

// Plan is what a target release would change.
type Plan struct {
	Current        string
	Target         string
	Breaking       []Breaking
	NotesAvailable bool
	CanApply       bool
	CannotApplyWhy string
	PublishedAt    time.Time
}

// MissingAcks returns the ids of breaking changes that need an
// acknowledgement and were not given one.
func (p Plan) MissingAcks(acked []string) []string {
	have := map[string]bool{}
	for _, id := range acked {
		have[id] = true
	}
	var out []string
	for _, b := range p.Breaking {
		if b.RequiresAck && !have[b.ID] {
			out = append(out, b.ID)
		}
	}
	return out
}

// NeedAckCount counts breaking changes that need an acknowledgement.
func (p Plan) NeedAckCount() int {
	n := 0
	for _, b := range p.Breaking {
		if b.RequiresAck {
			n++
		}
	}
	return n
}

// PreflightResult is a fresh evaluation of the upgrade preflight.
type PreflightResult struct {
	Checks  []upgrade.Check
	Blocked bool
}

// AttemptInfo is the outcome of a self-upgrade run.
type AttemptInfo struct {
	ID      string
	Outcome string
	Error   string
}

// Host is everything the scheduler needs from the running control plane.
// The API router implements it, so the scheduler uses the same release
// lookup, preflight and launcher a person clicking "Upgrade now" does.
type Host interface {
	// Latest is the newest release on the configured channel.
	Latest(ctx context.Context) (Candidate, bool, error)
	// Plan describes target, or returns ErrReleaseNotFound.
	Plan(ctx context.Context, target string) (Plan, error)
	// Preflight evaluates the checks for target, bypassing release caches.
	Preflight(ctx context.Context, target string) (PreflightResult, error)
	// BackupNow takes a control plane backup and returns its name.
	BackupNow(ctx context.Context) (string, error)
	// Apply starts the host upgrade.
	Apply(ctx context.Context, target string, ack []string, initiator string) error
	// Running reports whether a self-upgrade is in progress on the host.
	Running(ctx context.Context) bool
	// Attempt finds the newest recorded attempt started by initiator.
	Attempt(ctx context.Context, initiator string) (AttemptInfo, bool, error)
}

// Store is the persistence the scheduler uses. *store.DB satisfies it.
type Store interface {
	GetUpgradePolicy(ctx context.Context) (store.UpgradePolicy, error)
	SetUpgradeDecision(ctx context.Context, tag, decision, at string) error
	InsertUpgradeSchedule(ctx context.Context, s store.UpgradeSchedule) error
	ActiveUpgradeSchedule(ctx context.Context) (*store.UpgradeSchedule, error)
	ListUpgradeSchedules(ctx context.Context, limit int) ([]store.UpgradeSchedule, error)
	UpdateUpgradeSchedule(ctx context.Context, s store.UpgradeSchedule, from ...string) (bool, error)
}

// Notifier sends a plain-text notice to alert channels. An empty id list
// means every enabled channel, so an unconfigured policy still alerts.
type Notifier interface {
	Notify(ctx context.Context, channelIDs []string, text string) error
}

// Config holds the scheduler's tunables, all overridable from the environment.
type Config struct {
	// Grace is how long after a manual run_at the scheduler keeps retrying.
	Grace time.Duration
	// RetryInterval is the minimum gap between fire-time re-checks.
	RetryInterval time.Duration
	// StartTimeout is how long a fired upgrade may show no attempt before it
	// is reported as not started.
	StartTimeout time.Duration
	// MaxAhead bounds how far in the future a manual schedule may be.
	MaxAhead time.Duration
	// MinLead is the shortest time between now and a scheduled run.
	MinLead time.Duration
}

// DefaultConfig returns the defaults used when no environment override is set.
func DefaultConfig() Config {
	return Config{
		Grace: 2 * time.Hour, RetryInterval: 2 * time.Minute, StartTimeout: 10 * time.Minute,
		MaxAhead: 90 * 24 * time.Hour, MinLead: time.Minute,
	}
}

// ConfigFromEnv applies APP_UPGRADE_SCHEDULE_* overrides to the defaults.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	c := DefaultConfig()
	read := func(key string, dst *time.Duration) {
		if v, ok := lookup(key); ok {
			if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
				*dst = d
			}
		}
	}
	read("APP_UPGRADE_SCHEDULE_GRACE", &c.Grace)
	read("APP_UPGRADE_RETRY_INTERVAL", &c.RetryInterval)
	read("APP_UPGRADE_START_TIMEOUT", &c.StartTimeout)
	read("APP_UPGRADE_SCHEDULE_MAX_AHEAD", &c.MaxAhead)
	return c
}

// Scheduler fires scheduled upgrades and, when a policy allows, creates them.
type Scheduler struct {
	Store    Store
	Host     Host
	Windows  func(ctx context.Context) ([]alerting.MaintenanceWindow, error)
	Notifier Notifier
	Config   Config
	Logger   *slog.Logger
	Now      func() time.Time
	// Current returns the running version.
	Current func() string
	// Audit records a scheduler-initiated change in the audit log.
	Audit func(ctx context.Context, detail string)
	// NewID returns a schedule id.
	NewID func() string

	mu sync.Mutex
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Scheduler) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Scheduler) audit(ctx context.Context, format string, a ...any) {
	if s.Audit != nil {
		s.Audit(ctx, fmt.Sprintf(format, a...))
	}
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseStamp(v string) time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Tick advances the active schedule, or considers creating one under the
// automatic policy. It is safe to call at any time and any number of times.
func (s *Scheduler) Tick(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, err := s.Store.ActiveUpgradeSchedule(ctx)
	if err != nil {
		return fmt.Errorf("upgradesched: load active schedule: %w", err)
	}
	if active == nil {
		return s.considerAuto(ctx)
	}
	switch active.State {
	case store.UpgradeScheduleRunning:
		return s.reconcileRunning(ctx, *active)
	default:
		return s.maybeFire(ctx, *active)
	}
}

func (s *Scheduler) channelIDs(raw string) []string {
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

func (s *Scheduler) policyChannels(ctx context.Context) []string {
	row, err := s.Store.GetUpgradePolicy(ctx)
	if err != nil {
		return nil
	}
	return s.channelIDs(row.NotifyChannelIDs)
}

func (s *Scheduler) notify(ctx context.Context, text string) {
	if s.Notifier == nil {
		return
	}
	if err := s.Notifier.Notify(ctx, s.policyChannels(ctx), text); err != nil {
		s.log().Warn("upgrade scheduler: notice failed", slog.String("error", err.Error()))
	}
}

// finish moves sc to a terminal state exactly once and tells the operator.
func (s *Scheduler) finish(ctx context.Context, sc store.UpgradeSchedule, state, reason string, from ...string) {
	prev := sc.State
	sc.State, sc.LastReason, sc.FinishedAt = state, reason, stamp(s.now())
	changed, err := s.Store.UpdateUpgradeSchedule(ctx, sc, from...)
	if err != nil {
		s.log().Error("upgrade scheduler: finish failed", slog.String("schedule", sc.ID), slog.String("error", err.Error()))
		return
	}
	if !changed {
		return
	}
	s.log().Info("upgrade scheduler: finished", slog.String("schedule", sc.ID), slog.String("target", sc.TargetVersion),
		slog.String("from_state", prev), slog.String("state", state), slog.String("reason", reason))
	s.audit(ctx, "scheduled upgrade to %s %s: %s (schedule %s)", sc.TargetVersion, state, reason, sc.ID)
	switch state {
	case store.UpgradeScheduleApplied:
		s.notify(ctx, fmt.Sprintf("The control plane was upgraded to %s.", sc.TargetVersion))
	case store.UpgradeScheduleCancelled:
	default:
		s.notify(ctx, fmt.Sprintf("The scheduled upgrade to %s did not run (%s): %s.", sc.TargetVersion, state, reason))
	}
}

func (s *Scheduler) maybeFire(ctx context.Context, sc store.UpgradeSchedule) error {
	now := s.now()
	runAt, until := parseStamp(sc.RunAt), parseStamp(sc.RetryUntil)
	if now.Before(runAt) {
		return nil
	}
	from := []string{store.UpgradeSchedulePending, store.UpgradeScheduleBlocked}
	if now.After(until) {
		reason := "the maintenance window ended before the checks passed"
		if sc.LastReason != "" {
			reason += ": " + sc.LastReason
		}
		s.finish(ctx, sc, store.UpgradeScheduleMissed, reason, from...)
		return nil
	}
	if sc.State == store.UpgradeScheduleBlocked {
		if last := parseStamp(sc.LastCheckAt); !last.IsZero() && now.Sub(last) < s.Config.RetryInterval {
			return nil
		}
	}
	if cmp, ok := semver.Compare(s.Current(), sc.TargetVersion); ok && cmp >= 0 {
		s.finish(ctx, sc, store.UpgradeScheduleCancelled, "the control plane already runs "+s.Current(), from...)
		return nil
	}
	if s.Host.Running(ctx) {
		return s.block(ctx, sc, "another upgrade is in progress on this host", false)
	}

	var pol Policy
	if sc.Source == store.UpgradeSourceAuto {
		row, err := s.Store.GetUpgradePolicy(ctx)
		if err != nil {
			return fmt.Errorf("upgradesched: load policy: %w", err)
		}
		pol = PolicyFromStore(row, s.channelIDs(row.NotifyChannelIDs))
		if !pol.Active() {
			s.finish(ctx, sc, store.UpgradeScheduleCancelled, "automatic upgrades were turned off or paused", from...)
			return nil
		}
	}

	plan, err := s.Host.Plan(ctx, sc.TargetVersion)
	if errors.Is(err, ErrReleaseNotFound) {
		s.finish(ctx, sc, store.UpgradeScheduleFailed, sc.TargetVersion+" is no longer a release on the configured channel", from...)
		return nil
	}
	if err != nil {
		return s.block(ctx, sc, "could not read the release: "+err.Error(), true)
	}
	if !plan.CanApply {
		s.finish(ctx, sc, store.UpgradeScheduleFailed, "this host cannot apply upgrades from the control plane: "+plan.CannotApplyWhy, from...)
		return nil
	}
	var acked []string
	_ = json.Unmarshal([]byte(sc.AckedJSON), &acked)
	if missing := plan.MissingAcks(acked); len(missing) > 0 {
		reason := "breaking changes need acknowledgement: " + strings.Join(missing, ", ")
		state := store.UpgradeScheduleFailed
		if sc.Source == store.UpgradeSourceAuto {
			state = store.UpgradeScheduleCancelled
		}
		s.finish(ctx, sc, state, reason, from...)
		return nil
	}
	if CrossesMajor(s.Current(), sc.TargetVersion) && !sc.MajorAcked {
		s.finish(ctx, sc, store.UpgradeScheduleFailed, "a major version jump needs acknowledgement", from...)
		return nil
	}

	pre, err := s.Host.Preflight(ctx, sc.TargetVersion)
	if err != nil {
		return s.block(ctx, sc, "preflight could not run: "+err.Error(), true)
	}
	backedUp := false
	if pre.Blocked && onlyBackupBlocks(pre.Checks) {
		if _, err := s.Host.BackupNow(ctx); err != nil {
			return s.block(ctx, sc, "backup failed: "+err.Error(), true)
		}
		backedUp = true
		if pre, err = s.Host.Preflight(ctx, sc.TargetVersion); err != nil {
			return s.block(ctx, sc, "preflight could not run: "+err.Error(), true)
		}
	}
	if pre.Blocked {
		return s.block(ctx, sc, BlockerSummary(pre.Checks), true)
	}
	if !backedUp {
		if _, err := s.Host.BackupNow(ctx); err != nil && !errors.Is(err, ErrBackupNotAvail) {
			return s.block(ctx, sc, "backup failed: "+err.Error(), true)
		}
	}
	return s.launch(ctx, sc, acked, from)
}

func (s *Scheduler) launch(ctx context.Context, sc store.UpgradeSchedule, acked []string, from []string) error {
	prefix := "schedule:"
	if sc.Source == store.UpgradeSourceAuto {
		prefix = "auto:"
	}
	sc.State, sc.Initiator, sc.FiredAt, sc.LastReason = store.UpgradeScheduleRunning, prefix+sc.ID, stamp(s.now()), ""
	changed, err := s.Store.UpdateUpgradeSchedule(ctx, sc, from...)
	if err != nil {
		return fmt.Errorf("upgradesched: mark running: %w", err)
	}
	if !changed {
		return nil
	}
	s.audit(ctx, "scheduled upgrade to %s started (schedule %s)", sc.TargetVersion, sc.ID)
	if err := s.Host.Apply(ctx, sc.TargetVersion, acked, sc.Initiator); err != nil {
		s.log().Error("upgrade scheduler: start failed", slog.String("schedule", sc.ID), slog.String("error", err.Error()))
		s.finish(ctx, sc, store.UpgradeScheduleFailed, "could not start the upgrade on the host: "+err.Error(), store.UpgradeScheduleRunning)
		return nil
	}
	s.log().Info("upgrade scheduler: upgrade started", slog.String("schedule", sc.ID), slog.String("target", sc.TargetVersion))
	return nil
}

// block records a failed fire-time check and raises it once per distinct reason.
func (s *Scheduler) block(ctx context.Context, sc store.UpgradeSchedule, reason string, counts bool) error {
	prevReason := sc.LastReason
	wasBlocked := sc.State == store.UpgradeScheduleBlocked
	sc.State, sc.LastReason, sc.LastCheckAt = store.UpgradeScheduleBlocked, reason, stamp(s.now())
	if counts {
		sc.CheckCount++
	}
	changed, err := s.Store.UpdateUpgradeSchedule(ctx, sc, store.UpgradeSchedulePending, store.UpgradeScheduleBlocked)
	if err != nil {
		return fmt.Errorf("upgradesched: record blocked schedule: %w", err)
	}
	if changed && (!wasBlocked || prevReason != reason) && counts {
		s.log().Warn("upgrade scheduler: blocked at fire time", slog.String("schedule", sc.ID), slog.String("reason", reason))
		s.audit(ctx, "scheduled upgrade to %s blocked: %s (schedule %s)", sc.TargetVersion, reason, sc.ID)
		s.notify(ctx, fmt.Sprintf("The scheduled upgrade to %s was not applied: %s. It will retry until %s.", sc.TargetVersion, reason, sc.RetryUntil))
	}
	return nil
}

func (s *Scheduler) reconcileRunning(ctx context.Context, sc store.UpgradeSchedule) error {
	att, found, err := s.Host.Attempt(ctx, sc.Initiator)
	if err != nil {
		return fmt.Errorf("upgradesched: look up attempt: %w", err)
	}
	from := store.UpgradeScheduleRunning
	switch {
	case found && att.Outcome == "succeeded":
		s.finish(ctx, sc, store.UpgradeScheduleApplied, "upgrade completed", from)
	case found && att.Outcome == "running":
	case found:
		reason := att.Outcome
		if att.Error != "" {
			reason += ": " + att.Error
		}
		s.finish(ctx, sc, store.UpgradeScheduleFailed, reason, from)
	default:
		if cmp, ok := semver.Compare(s.Current(), sc.TargetVersion); ok && cmp >= 0 {
			s.finish(ctx, sc, store.UpgradeScheduleApplied, "upgrade completed", from)
			return nil
		}
		if fired := parseStamp(sc.FiredAt); !fired.IsZero() && s.now().Sub(fired) > s.Config.StartTimeout && !s.Host.Running(ctx) {
			s.finish(ctx, sc, store.UpgradeScheduleFailed, "the upgrade did not start on the host", from)
		}
	}
	return nil
}

func onlyBackupBlocks(checks []upgrade.Check) bool {
	found := false
	for _, c := range checks {
		if c.Status != upgrade.StatusFail && c.Status != upgrade.StatusPending {
			continue
		}
		found = true
		if c.Fix == nil || c.Fix.Action != upgrade.FixBackupNow {
			return false
		}
	}
	return found
}

// BlockerSummary renders the blocking checks as one line.
func BlockerSummary(checks []upgrade.Check) string {
	var parts []string
	for _, c := range checks {
		if c.Status == upgrade.StatusFail || c.Status == upgrade.StatusPending {
			parts = append(parts, c.Name+": "+c.Message)
		}
	}
	if len(parts) == 0 {
		return "preflight is blocked"
	}
	return strings.Join(parts, "; ")
}
