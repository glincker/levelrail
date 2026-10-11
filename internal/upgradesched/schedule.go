package upgradesched

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/semver"
)

// AckError lists the breaking changes a request left unacknowledged.
type AckError struct{ IDs []string }

func (e *AckError) Error() string {
	return ErrNeedsAck.Error() + ": " + strings.Join(e.IDs, ", ")
}

// Is lets errors.Is match ErrNeedsAck.
func (e *AckError) Is(target error) bool { return target == ErrNeedsAck }

// Request asks for one upgrade at a time.
type Request struct {
	// Target is the release tag, empty for the newest on the channel.
	Target string
	// At is the earliest run time. Ignored when Window is set.
	At time.Time
	// Window runs in the next maintenance window instead of at At.
	Window bool
	// Timezone is the zone the operator thinks in, recorded for display.
	Timezone string
	Ack      []string
	MajorAck bool
	// Replace cancels a pending or blocked schedule first.
	Replace bool
	Actor   string
}

// Schedule validates req against the target release and stores it.
func (s *Scheduler) Schedule(ctx context.Context, req Request) (store.UpgradeSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	target := strings.TrimSpace(req.Target)
	if target == "" {
		latest, ok, err := s.Host.Latest(ctx)
		if err != nil {
			return store.UpgradeSchedule{}, fmt.Errorf("upgradesched: look up latest release: %w", err)
		}
		if !ok {
			return store.UpgradeSchedule{}, fmt.Errorf("%w: the latest release could not be fetched", ErrInvalid)
		}
		target = latest.Tag
	}
	if cmp, ok := semver.Compare(target, s.Current()); ok && cmp <= 0 {
		return store.UpgradeSchedule{}, fmt.Errorf("%w: %s is not newer than the running version", ErrInvalid, target)
	}
	plan, err := s.Host.Plan(ctx, target)
	if err != nil {
		return store.UpgradeSchedule{}, err
	}
	if !plan.CanApply {
		return store.UpgradeSchedule{}, fmt.Errorf("%w: this host cannot apply upgrades from the control plane: %s", ErrInvalid, plan.CannotApplyWhy)
	}
	if !plan.NotesAvailable {
		return store.UpgradeSchedule{}, fmt.Errorf("%w: release notes could not be fetched, so breaking changes cannot be checked", ErrInvalid)
	}
	if missing := plan.MissingAcks(req.Ack); len(missing) > 0 {
		return store.UpgradeSchedule{}, &AckError{IDs: missing}
	}
	if CrossesMajor(s.Current(), target) && !req.MajorAck {
		return store.UpgradeSchedule{}, ErrNeedsMajorAck
	}

	runAt, until, mode, err := s.resolveTime(ctx, req, now)
	if err != nil {
		return store.UpgradeSchedule{}, err
	}
	acked := req.Ack
	if acked == nil {
		acked = []string{}
	}
	rawAck, _ := json.Marshal(acked)
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	sc := store.UpgradeSchedule{
		ID: s.NewID(), TargetVersion: target, Source: store.UpgradeSourceManual, Mode: mode,
		RunAt: stamp(runAt), RetryUntil: stamp(until), Timezone: tz, AckedJSON: string(rawAck),
		MajorAcked: req.MajorAck, CreatedBy: req.Actor, CreatedAt: stamp(now),
	}
	if req.Replace {
		if err := s.cancelActiveLocked(ctx, req.Actor, "replaced by a new schedule"); err != nil && !errors.Is(err, ErrNothingScheduled) {
			return store.UpgradeSchedule{}, err
		}
	}
	if err := s.Store.InsertUpgradeSchedule(ctx, sc); err != nil {
		if errors.Is(err, store.ErrUpgradeScheduleActive) {
			return store.UpgradeSchedule{}, ErrAlreadyScheduled
		}
		return store.UpgradeSchedule{}, fmt.Errorf("upgradesched: save schedule: %w", err)
	}
	sc.State = store.UpgradeSchedulePending
	s.audit(ctx, "upgrade to %s scheduled for %s by %s (schedule %s)", target, sc.RunAt, req.Actor, sc.ID)
	s.log().Info("upgrade scheduler: scheduled", slog.String("schedule", sc.ID), slog.String("target", target), slog.String("run_at", sc.RunAt))
	return sc, nil
}

func (s *Scheduler) resolveTime(ctx context.Context, req Request, now time.Time) (time.Time, time.Time, string, error) {
	if !req.Window {
		at := req.At.UTC()
		if at.IsZero() {
			return time.Time{}, time.Time{}, "", fmt.Errorf("%w: a run time or a maintenance window is required", ErrInvalid)
		}
		if at.Before(now.Add(s.Config.MinLead)) {
			return time.Time{}, time.Time{}, "", fmt.Errorf("%w: the run time must be in the future", ErrInvalid)
		}
		if at.After(now.Add(s.Config.MaxAhead)) {
			return time.Time{}, time.Time{}, "", fmt.Errorf("%w: the run time is more than %s away", ErrInvalid, s.Config.MaxAhead)
		}
		return at, at.Add(s.Config.Grace), store.UpgradeModeAt, nil
	}
	span, ok, err := s.NextWindow(ctx, now)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	if !ok {
		return time.Time{}, time.Time{}, "", ErrNoWindow
	}
	return span.Start, span.End, store.UpgradeModeWindow, nil
}

// NextWindow picks the earliest opening among the upgrade policy's window
// and the enabled alert maintenance windows that cover everything.
func (s *Scheduler) NextWindow(ctx context.Context, now time.Time) (Span, bool, error) {
	var spans []Span
	if row, err := s.Store.GetUpgradePolicy(ctx); err == nil && row.WindowCron != "" {
		w := Window{Cron: row.WindowCron, Duration: time.Duration(row.WindowDurationSeconds) * time.Second, Timezone: row.WindowTimezone}
		if sp, ok, err := w.Next(now); err == nil && ok {
			spans = append(spans, sp)
		}
	}
	if len(spans) == 0 && s.Windows != nil {
		list, err := s.Windows(ctx)
		if err != nil {
			return Span{}, false, fmt.Errorf("upgradesched: list maintenance windows: %w", err)
		}
		for _, mw := range list {
			if !mw.Enabled || mw.Scope != alerting.ScopeAll {
				continue
			}
			if sp, ok, err := (Window{Cron: mw.Cron, Duration: mw.Duration, Timezone: mw.Timezone}).Next(now); err == nil && ok {
				spans = append(spans, sp)
			}
		}
	}
	if len(spans) == 0 {
		return Span{}, false, nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start.Before(spans[j].Start) })
	return spans[0], true, nil
}

// Cancel withdraws the active schedule unless it is already running.
func (s *Scheduler) Cancel(ctx context.Context, actor string) (store.UpgradeSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, err := s.Store.ActiveUpgradeSchedule(ctx)
	if err != nil {
		return store.UpgradeSchedule{}, fmt.Errorf("upgradesched: load active schedule: %w", err)
	}
	if active == nil {
		return store.UpgradeSchedule{}, ErrNothingScheduled
	}
	if err := s.cancelActiveLocked(ctx, actor, "cancelled by "+actor); err != nil {
		return store.UpgradeSchedule{}, err
	}
	active.State = store.UpgradeScheduleCancelled
	return *active, nil
}

func (s *Scheduler) cancelActiveLocked(ctx context.Context, actor, reason string) error {
	active, err := s.Store.ActiveUpgradeSchedule(ctx)
	if err != nil {
		return fmt.Errorf("upgradesched: load active schedule: %w", err)
	}
	if active == nil {
		return ErrNothingScheduled
	}
	if active.State == store.UpgradeScheduleRunning {
		return ErrRunning
	}
	sc := *active
	sc.State, sc.LastReason, sc.FinishedAt = store.UpgradeScheduleCancelled, reason, stamp(s.now())
	changed, err := s.Store.UpdateUpgradeSchedule(ctx, sc, store.UpgradeSchedulePending, store.UpgradeScheduleBlocked)
	if err != nil {
		return fmt.Errorf("upgradesched: cancel schedule: %w", err)
	}
	if !changed {
		return ErrRunning
	}
	s.audit(ctx, "scheduled upgrade to %s cancelled by %s (schedule %s)", sc.TargetVersion, actor, sc.ID)
	return nil
}

// considerAuto creates a schedule from the automatic policy when the newest
// release qualifies. Every verdict is recorded so the dashboard can say why a
// release is being held.
func (s *Scheduler) considerAuto(ctx context.Context) error {
	row, err := s.Store.GetUpgradePolicy(ctx)
	if err != nil {
		return fmt.Errorf("upgradesched: load policy: %w", err)
	}
	pol := PolicyFromStore(row, s.channelIDs(row.NotifyChannelIDs))
	if pol.Mode == ModeOff {
		return nil
	}
	cand, ok, err := s.Host.Latest(ctx)
	if err != nil {
		return fmt.Errorf("upgradesched: look up latest release: %w", err)
	}
	if !ok {
		return nil
	}
	if cmp, ok := semver.Compare(cand.Tag, s.Current()); !ok || cmp <= 0 {
		return nil
	}
	plan, err := s.Host.Plan(ctx, cand.Tag)
	if err != nil {
		if errors.Is(err, ErrReleaseNotFound) {
			return nil
		}
		return fmt.Errorf("upgradesched: plan %s: %w", cand.Tag, err)
	}
	if cand.PublishedAt.IsZero() {
		cand.PublishedAt = plan.PublishedAt
	}
	now := s.now()
	v := Eligible(pol, s.Current(), cand, plan.NeedAckCount(), plan.NotesAvailable, now)
	if !v.OK {
		loud := v.Code == VerdictBreaking || v.Code == VerdictMajor || v.Code == VerdictNoNotes
		s.decide(ctx, row, cand.Tag, "held: "+v.Reason, loud)
		return nil
	}
	if !plan.CanApply {
		s.decide(ctx, row, cand.Tag, "held: this host cannot apply upgrades from the control plane: "+plan.CannotApplyWhy, false)
		return nil
	}
	if s.priorAutoFailed(ctx, cand.Tag) {
		s.decide(ctx, row, cand.Tag, "held: an automatic upgrade to this release failed earlier, apply it manually", false)
		return nil
	}
	span, ok, err := pol.Window.Next(now)
	if err != nil || !ok {
		s.decide(ctx, row, cand.Tag, "held: the maintenance window has no upcoming opening", false)
		return nil
	}
	sc := store.UpgradeSchedule{
		ID: s.NewID(), TargetVersion: cand.Tag, Source: store.UpgradeSourceAuto, Mode: store.UpgradeModeWindow,
		RunAt: stamp(span.Start), RetryUntil: stamp(span.End), Timezone: pol.Window.Timezone,
		AckedJSON: "[]", CreatedBy: "automatic upgrades", CreatedAt: stamp(now),
	}
	if err := s.Store.InsertUpgradeSchedule(ctx, sc); err != nil {
		if errors.Is(err, store.ErrUpgradeScheduleActive) {
			return nil
		}
		return fmt.Errorf("upgradesched: save automatic schedule: %w", err)
	}
	s.audit(ctx, "automatic upgrade to %s scheduled for %s (schedule %s)", cand.Tag, sc.RunAt, sc.ID)
	s.decide(ctx, row, cand.Tag, "scheduled for "+sc.RunAt, true)
	return nil
}

func (s *Scheduler) priorAutoFailed(ctx context.Context, tag string) bool {
	list, err := s.Store.ListUpgradeSchedules(ctx, 20)
	if err != nil {
		return false
	}
	for _, sc := range list {
		if sc.Source == store.UpgradeSourceAuto && sc.TargetVersion == tag && sc.State == store.UpgradeScheduleFailed {
			return true
		}
	}
	return false
}

func (s *Scheduler) decide(ctx context.Context, row store.UpgradePolicy, tag, text string, loud bool) {
	if row.DecisionTag == tag && row.Decision == text {
		return
	}
	if err := s.Store.SetUpgradeDecision(ctx, tag, text, stamp(s.now())); err != nil {
		s.log().Warn("upgrade scheduler: record decision failed", slog.String("error", err.Error()))
		return
	}
	s.log().Info("upgrade scheduler: automatic decision", slog.String("target", tag), slog.String("decision", text))
	if loud {
		s.notify(ctx, fmt.Sprintf("Automatic upgrades, %s: %s.", tag, text))
	}
}
