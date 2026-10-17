package alerting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// DefaultBackupMissingGracePeriod is the slack past a backup's expected interval before it counts as overdue.
const DefaultBackupMissingGracePeriod = 6 * time.Hour

// Bounded so the history query stays cheap on every tick.
const backupMissingHistoryLookback = 20

// BackupSource is the narrow store surface a backup-missing evaluation needs; *store.DB satisfies it.
type BackupSource interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	ListBackupHistory(ctx context.Context, databaseName string, limit int, before *time.Time) ([]store.BackupHistory, error)
	GetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName string) (store.ServiceVolumeBackupConfig, error)
	ListServiceVolumeBackupHistory(ctx context.Context, serviceName, volumeName string, limit int, before *time.Time) ([]store.BackupHistory, error)
}

// EvaluateBackupMissing runs one KindBackupMissing rule and returns its updated state and, when firing, a notice line.
// Missing is measured against the same cron schedule internal/backup.Scheduler reads.
func EvaluateBackupMissing(ctx context.Context, source BackupSource, r Rule, defaultGracePeriod time.Duration, now time.Time) (Rule, string, error) {
	gracePeriod := r.ForDuration
	if gracePeriod <= 0 {
		gracePeriod = defaultGracePeriod
	}
	if gracePeriod <= 0 {
		gracePeriod = DefaultBackupMissingGracePeriod
	}

	next := r
	next.LastEvaluatedAt = &now

	label, schedule, history, err := loadBackupMissingTarget(ctx, source, r)
	if err != nil {
		return r, "", err
	}
	if schedule == "" {
		// Not currently scheduled (never configured, or cleared): nothing
		// expected, so nothing to be missing.
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, "", nil
	}
	if len(history) == 0 {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, "", nil
	}

	overdue, anchor, anchorIsSuccess, err := BackupOverdue(history, schedule, gracePeriod, now)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}
	firing := overdue

	v := now.Sub(anchor).Hours()
	next.LastValue = &v

	var notice string
	if firing {
		notice = backupMissingNotice(label, history[0].Status, anchor, anchorIsSuccess, now)
	}

	return advanceState(next, r, firing, 0, now), notice, nil
}

// BackupOverdue reports whether the newest history (newest first, non-empty)
// trails its cron schedule by more than one expected interval plus grace. A
// target that never succeeded anchors on its oldest attempt.
func BackupOverdue(history []store.BackupHistory, schedule string, grace time.Duration, now time.Time) (overdue bool, anchor time.Time, anchorIsSuccess bool, err error) {
	sched, perr := cronexpr.Parse(schedule)
	if perr != nil {
		return false, time.Time{}, false, fmt.Errorf("parse backup schedule %q: %w", schedule, perr)
	}
	anchor, anchorIsSuccess = latestSucceededBackupTime(history)
	if anchor.IsZero() {
		anchor, err = parseBackupHistoryTime(history[len(history)-1].StartedAt)
		if err != nil {
			return false, time.Time{}, false, fmt.Errorf("parse backup history started_at: %w", err)
		}
	}
	t1 := sched.Next(anchor)
	interval := sched.Next(t1).Sub(t1)
	return now.After(anchor.Add(interval).Add(grace)), anchor, anchorIsSuccess, nil
}

// An unconfigured target goes quiet rather than erroring, like a deleted scheduled task.
func loadBackupMissingTarget(ctx context.Context, source BackupSource, r Rule) (label, schedule string, history []store.BackupHistory, err error) {
	if r.BackupResourceKind == store.BackupResourceKindVolume {
		label = r.BackupServiceName + "/" + r.BackupVolumeName
		cfg, cerr := source.GetServiceVolumeBackupSchedule(ctx, r.BackupServiceName, r.BackupVolumeName)
		if errors.Is(cerr, store.ErrServiceVolumeBackupNotFound) {
			return label, "", nil, nil
		}
		if cerr != nil {
			return "", "", nil, fmt.Errorf("alerting: evaluate rule %q: load service volume backup schedule: %w", r.ID, cerr)
		}
		history, err = source.ListServiceVolumeBackupHistory(ctx, r.BackupServiceName, r.BackupVolumeName, backupMissingHistoryLookback, nil)
		if err != nil {
			return "", "", nil, fmt.Errorf("alerting: evaluate rule %q: list service volume backup history: %w", r.ID, err)
		}
		return label, cfg.BackupSchedule, history, nil
	}

	label = r.BackupDatabaseName
	d, derr := source.GetDesiredDatabase(ctx, r.BackupDatabaseName)
	if errors.Is(derr, store.ErrDatabaseNotFound) {
		return label, "", nil, nil
	}
	if derr != nil {
		return "", "", nil, fmt.Errorf("alerting: evaluate rule %q: load database %q: %w", r.ID, r.BackupDatabaseName, derr)
	}
	history, err = source.ListBackupHistory(ctx, r.BackupDatabaseName, backupMissingHistoryLookback, nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("alerting: evaluate rule %q: list backup history: %w", r.ID, err)
	}
	return label, d.BackupSchedule, history, nil
}

// A malformed timestamp counts as no success rather than failing the whole evaluation.
func latestSucceededBackupTime(history []store.BackupHistory) (t time.Time, ok bool) {
	for _, h := range history {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		parsed, err := parseBackupHistoryTime(h.StartedAt)
		if err != nil {
			continue
		}
		return parsed, true
	}
	return time.Time{}, false
}

// Reads back the RFC3339 UTC layout internal/backup writes.
func parseBackupHistoryTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339, raw)
}

// Distinguishes still-failing from never-attempted in the notice text.
func backupMissingNotice(label, lastAttemptStatus string, anchor time.Time, anchorIsSuccess bool, now time.Time) string {
	age := now.Sub(anchor).Round(time.Minute)
	if !anchorIsSuccess {
		return fmt.Sprintf("%s: no successful backup in the last %s (every recent attempt failed, most recent status %q)", label, age, lastAttemptStatus)
	}
	if lastAttemptStatus == store.BackupStatusFailed {
		return fmt.Sprintf("%s: last successful backup was %s ago and the most recent attempt since then failed", label, age)
	}
	return fmt.Sprintf("%s: last successful backup was %s ago, later than its schedule expects", label, age)
}
