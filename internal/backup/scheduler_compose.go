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
	return errors.Join(errs...)
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
