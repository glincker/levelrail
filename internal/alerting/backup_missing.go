package alerting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultBackupMissingGracePeriod is how long a database's or service
// volume's last successful backup can trail its schedule's own expected
// interval before EvaluateBackupMissing treats it as overdue rather than
// evaluation-tick jitter, when a rule's own ForDuration (reused as this
// grace period, see Rule's own doc comment) is unset. Matches
// DefaultCertRenewalStalledThreshold's own value: a scheduled backup, like
// a certificate renewal, should already be long done by the time its own
// expected interval elapses, so several hours of slack past that is a
// genuine anomaly, not noise.
const DefaultBackupMissingGracePeriod = 6 * time.Hour

// backupMissingHistoryLookback bounds how many of a target's most recent
// backup_history rows EvaluateBackupMissing reads to find the latest
// succeeded attempt: enough to see past a run of recent failures without
// an unbounded query on every tick.
const backupMissingHistoryLookback = 20

// BackupSource is the narrow store surface a backup-missing evaluation
// needs: the current schedule config for a database or service volume,
// and enough recent history to find its last successful run. *store.DB
// satisfies this structurally.
type BackupSource interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	ListBackupHistory(ctx context.Context, databaseName string, limit int, before *time.Time) ([]store.BackupHistory, error)
	GetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName string) (store.ServiceVolumeBackupConfig, error)
	ListServiceVolumeBackupHistory(ctx context.Context, serviceName, volumeName string, limit int, before *time.Time) ([]store.BackupHistory, error)
}

// EvaluateBackupMissing runs one KindBackupMissing rule and returns its
// updated evaluation state plus, when firing, a human-readable notice
// line for Engine to attach to the outgoing Event.
//
// "Missing" is measured against the same cron schedule
// internal/backup.Scheduler itself reads (store.DesiredDatabase.
// BackupSchedule or store.ServiceVolumeBackupConfig.BackupSchedule), not
// a second, independently configured cadence: the interval between two
// consecutive scheduled fire times from now (via cronexpr.Schedule.Next,
// called twice) stands in for that schedule's period, since nothing in
// this codebase persists an "expected next run" outside Scheduler's own
// in-memory map (that struct's own doc comment). The rule fires once now
// is more than that period plus its grace period (r.ForDuration when
// set, reused here per Rule's own doc comment, else defaultGracePeriod)
// past the last successful attempt.
//
// A target with no schedule currently configured, or with no history at
// all yet (a schedule just created, nothing had a chance to run), goes
// quiet rather than firing: there is nothing yet to compare against, the
// same "nothing to alert on" quiet-state EvaluateScheduledTaskFailure's
// own doc comment applies to a deleted watched task. A target with
// attempts but no success anywhere in the lookback window anchors the
// overdue calculation on its oldest known attempt instead, so a backup
// that has only ever failed still reads as overdue rather than silently
// never firing.
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

	sched, perr := cronexpr.Parse(schedule)
	if perr != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: parse backup schedule %q: %w", r.ID, schedule, perr)
	}

	anchor, anchorIsSuccess := latestSucceededBackupTime(history)
	if anchor.IsZero() {
		// No success anywhere in the lookback window: anchor on the
		// oldest known attempt instead, so this still reads as overdue
		// rather than never firing.
		anchor, err = parseBackupHistoryTime(history[len(history)-1].StartedAt)
		if err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: parse backup history started_at: %w", r.ID, err)
		}
	}

	t1 := sched.Next(anchor)
	interval := sched.Next(t1).Sub(t1)
	deadline := anchor.Add(interval).Add(gracePeriod)
	firing := now.After(deadline)

	v := now.Sub(anchor).Hours()
	next.LastValue = &v

	var notice string
	if firing {
		notice = backupMissingNotice(label, history[0].Status, anchor, anchorIsSuccess, now)
	}

	return advanceState(next, r, firing, 0, now), notice, nil
}

// loadBackupMissingTarget resolves r's watched database or service
// volume into a display label, its currently configured schedule (""
// meaning not scheduled), and its recent backup_history rows (newest
// first, empty when never attempted). An unconfigured target (database
// deleted, or a volume's schedule row never written) returns a ""
// schedule and no error, the same "watched thing is gone, go quiet"
// handling EvaluateScheduledTaskFailure gives a deleted task.
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

// latestSucceededBackupTime scans history (newest first) for the first
// BackupStatusSucceeded row and returns its parsed StartedAt; the zero
// time and false if none succeeded in the window, or if a succeeded
// row's StartedAt fails to parse (logged nowhere here, treated the same
// as "no success found" since EvaluateBackupMissing's own caller already
// wraps every hard error path; a single malformed timestamp among many
// rows should not fail the whole evaluation).
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

// parseBackupHistoryTime parses a backup_history StartedAt value:
// internal/backup.Runner writes it via time.RFC3339 in UTC
// (store.PruneBackupHistory's own doc comment), so this reads it back
// with the identical layout.
func parseBackupHistoryTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339, raw)
}

// backupMissingNotice builds the one-line summary Engine attaches to a
// firing backup_missing Event: label identifies the database or
// "service/volume" pair, lastAttemptStatus is the most recent attempt's
// own status (surfacing "still failing" distinctly from "hasn't even
// been attempted"), and age is anchor's distance from now.
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
