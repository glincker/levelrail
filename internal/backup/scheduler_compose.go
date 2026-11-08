package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// ComposeServiceScheduleStore is the optional store surface for compose database service schedules.
type ComposeServiceScheduleStore interface {
	ListScheduledComposeServices(ctx context.Context) ([]store.ComposeServiceBackup, error)
	PruneComposeServiceBackupRuns(ctx context.Context, app, service string, keep int, olderThan time.Time) ([]store.PrunedBackup, error)
}

// ScheduledComposeServiceRunner is *ComposeServiceRunner's RunComposeServiceBackup.
type ScheduledComposeServiceRunner interface {
	RunComposeServiceBackup(ctx context.Context, runID, app, service, engine, targetID string) error
}

// ComposeAppChecker is the optional surface the scheduler uses to skip schedules of deleted apps.
type ComposeAppChecker interface {
	ComposeAppExists(ctx context.Context, app string) (bool, error)
}

func composeScheduleKey(app, service string) string { return app + "/" + service }

// tickComposeServices fires due compose database service schedules. Called with s.mu held.
func (s *Scheduler) tickComposeServices(ctx context.Context, now time.Time) error {
	cs, ok := s.Store.(ComposeServiceScheduleStore)
	if !ok || s.ComposeServices == nil {
		return nil
	}
	rows, err := cs.ListScheduledComposeServices(ctx)
	if err != nil {
		return fmt.Errorf("backup: scheduler: list scheduled compose services: %w", err)
	}
	if s.composeNextRun == nil {
		s.composeNextRun = make(map[string]time.Time)
	}

	seen := make(map[string]struct{}, len(rows))
	var errs []error
	for _, c := range rows {
		key := composeScheduleKey(c.App, c.Service)
		seen[key] = struct{}{}
		if s.composeAppGone(ctx, c.App, key) {
			continue
		}
		sched, perr := cronexpr.Parse(c.BackupSchedule)
		if perr != nil {
			s.log().Warn("backup: scheduler: invalid compose service schedule, skipping",
				slog.String("service_ref", key), slog.String("error", perr.Error()))
			continue
		}
		next, known := s.composeNextRun[key]
		if !known || next.IsZero() {
			s.arm(s.composeNextRun, key, key, sched, now, !known)
			continue
		}
		if !cronexpr.Due(next, now) {
			continue
		}
		s.arm(s.composeNextRun, key, key, sched, now, true)
		if rerr := s.runScheduledCompose(ctx, cs, c); rerr != nil {
			errs = append(errs, fmt.Errorf("compose service %q: %w", key, rerr))
		}
	}
	for key := range s.composeNextRun {
		if _, ok := seen[key]; !ok {
			delete(s.composeNextRun, key)
		}
	}
	for key := range s.composeOrphanLogged {
		if _, ok := seen[key]; !ok {
			delete(s.composeOrphanLogged, key)
		}
	}
	return errors.Join(errs...)
}

// composeAppGone is a safety net for schedule rows left behind by a deleted app. It logs once per schedule.
func (s *Scheduler) composeAppGone(ctx context.Context, app, key string) bool {
	ck, ok := s.Store.(ComposeAppChecker)
	if !ok {
		return false
	}
	exists, err := ck.ComposeAppExists(ctx, app)
	if err != nil || exists {
		return false
	}
	if s.composeOrphanLogged == nil {
		s.composeOrphanLogged = make(map[string]struct{})
	}
	if _, done := s.composeOrphanLogged[key]; !done {
		s.composeOrphanLogged[key] = struct{}{}
		s.log().Warn("backup: scheduler: compose service schedule belongs to a deleted app, skipping",
			slog.String("service_ref", key))
	}
	delete(s.composeNextRun, key)
	return true
}

func (s *Scheduler) runScheduledCompose(ctx context.Context, cs ComposeServiceScheduleStore, c store.ComposeServiceBackup) error {
	label := composeScheduleKey(c.App, c.Service)
	runID, err := RandomComposeRunID()
	if err != nil {
		return err
	}
	if runErr := s.ComposeServices.RunComposeServiceBackup(ctx, runID, c.App, c.Service, c.Engine, c.BackupTargetID); runErr != nil {
		s.log().Error("backup: scheduled compose service run failed",
			slog.String("service_ref", label), slog.String("id", runID), slog.String("error", runErr.Error()))
		return runErr
	}
	keep, olderThan, ok := s.retentionRule(ctx, c.BackupRetain, c.BackupRetainDays)
	if !ok {
		return nil
	}
	pruned, pruneErr := cs.PruneComposeServiceBackupRuns(ctx, c.App, c.Service, keep, olderThan)
	if pruneErr != nil {
		s.log().Error("backup: scheduled compose retention prune failed",
			slog.String("service_ref", label), slog.String("error", pruneErr.Error()))
		return nil
	}
	if len(pruned) > 0 {
		s.deletePrunedObjects(ctx, label, pruned)
	}
	return nil
}
