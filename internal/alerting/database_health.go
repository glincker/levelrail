package alerting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Database rule kinds. They target BackupDatabaseName and reuse Threshold (percent) and ForDuration, so they need no schema change.
const (
	KindDatabaseConnections   Kind = "database_connections_high"
	KindDatabaseDisk          Kind = "database_disk_high"
	KindDatabaseBackupFailing Kind = "database_backup_failing"
	KindDatabaseDown          Kind = "database_down"
	KindDatabaseBackupStale   Kind = "database_backup_stale"
)

// DefaultDatabaseBackupStaleHours is the age of the newest successful backup past which database_backup_stale fires when Threshold is unset.
const DefaultDatabaseBackupStaleHours = 26.0

// Defaults when a rule leaves Threshold unset.
const (
	DefaultDatabaseConnectionsPercent = 80.0
	DefaultDatabaseDiskPercent        = 85.0
)

// ErrDatabaseHealthUnavailable means the probe cannot read the database right now (stopped, remote node, unsupported engine); the rule goes quiet.
var ErrDatabaseHealthUnavailable = errors.New("alerting: database health unavailable")

// IsDatabaseKind reports whether k is one of the database rule kinds.
func IsDatabaseKind(k Kind) bool {
	switch k {
	case KindDatabaseConnections, KindDatabaseDisk, KindDatabaseBackupFailing, KindDatabaseDown, KindDatabaseBackupStale, KindBackupFailed:
		return true
	}
	return false
}

// DatabaseHealthSource reads live connection and disk figures from a managed database.
type DatabaseHealthSource interface {
	DatabaseConnections(ctx context.Context, name string) (used, limit int, err error)
	DatabaseDiskUsedPercent(ctx context.Context, name string) (float64, error)
}

// DatabaseLivenessSource is an optional extension of DatabaseHealthSource that lets database_down rules probe reachability.
type DatabaseLivenessSource interface {
	DatabaseUp(ctx context.Context, name string) (bool, error)
}

// DatabaseBackupSource is the slice of the store the backup-failing rule reads.
type DatabaseBackupSource interface {
	ListBackupHistory(ctx context.Context, databaseName string, limit int, before *time.Time) ([]store.BackupHistory, error)
}

// EvaluateDatabaseRule runs one database rule and returns its new state and, when firing, a notice line.
func EvaluateDatabaseRule(ctx context.Context, health DatabaseHealthSource, backups DatabaseBackupSource, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	name := r.BackupDatabaseName

	switch r.Kind {
	case KindDatabaseConnections:
		if health == nil {
			return quietDatabaseRule(next), "", nil
		}
		used, limit, err := health.DatabaseConnections(ctx, name)
		if errors.Is(err, ErrDatabaseHealthUnavailable) || err == nil && limit <= 0 {
			return quietDatabaseRule(next), "", nil
		}
		if err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: read connections of %q: %w", r.ID, name, err)
		}
		pct := float64(used) / float64(limit) * 100
		at := thresholdOr(r.Threshold, DefaultDatabaseConnectionsPercent)
		out, firing := percentRule(next, r, pct, at, now)
		if !firing {
			return out, "", nil
		}
		return out, fmt.Sprintf("%s: %d of %d connections in use (%.0f%%, alert at %.0f%%)", name, used, limit, pct, at), nil
	case KindDatabaseDisk:
		if health == nil {
			return quietDatabaseRule(next), "", nil
		}
		pct, err := health.DatabaseDiskUsedPercent(ctx, name)
		if errors.Is(err, ErrDatabaseHealthUnavailable) {
			return quietDatabaseRule(next), "", nil
		}
		if err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: read disk of %q: %w", r.ID, name, err)
		}
		at := thresholdOr(r.Threshold, DefaultDatabaseDiskPercent)
		out, firing := percentRule(next, r, pct, at, now)
		if !firing {
			return out, "", nil
		}
		return out, fmt.Sprintf("%s: data disk %.0f%% full (alert at %.0f%%)", name, pct, at), nil
	case KindDatabaseDown:
		live, ok := health.(DatabaseLivenessSource)
		if !ok || health == nil {
			return quietDatabaseRule(next), "", nil
		}
		up, err := live.DatabaseUp(ctx, name)
		if errors.Is(err, ErrDatabaseHealthUnavailable) {
			return quietDatabaseRule(next), "", nil
		}
		if err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: probe %q: %w", r.ID, name, err)
		}
		v := 0.0
		if !up {
			v = 1
		}
		next.LastValue = &v
		next = advanceState(next, r, !up, r.ForDuration, now)
		if up {
			return next, "", nil
		}
		return next, fmt.Sprintf("%s: database is not accepting connections", name), nil
	case KindDatabaseBackupStale:
		if backups == nil {
			return quietDatabaseRule(next), "", nil
		}
		history, err := backups.ListBackupHistory(ctx, name, 50, nil)
		if err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: list backup history of %q: %w", r.ID, name, err)
		}
		maxAge := time.Duration(thresholdOr(r.Threshold, DefaultDatabaseBackupStaleHours) * float64(time.Hour))
		age, found := newestSuccessAge(history, now)
		stale := found && age > maxAge
		v := 0.0
		if found {
			v = age.Hours()
		}
		next.LastValue = &v
		next = advanceState(next, r, stale, r.ForDuration, now)
		if !stale {
			return next, "", nil
		}
		return next, fmt.Sprintf("%s: last successful backup was %.0fh ago (alert at %.0fh)", name, age.Hours(), maxAge.Hours()), nil
	case KindDatabaseBackupFailing:
		if backups == nil {
			return quietDatabaseRule(next), "", nil
		}
		need := r.RestartCountThreshold
		if need <= 0 {
			need = 1
		}
		history, err := backups.ListBackupHistory(ctx, name, need, nil)
		if err != nil {
			return r, "", fmt.Errorf("alerting: evaluate rule %q: list backup history of %q: %w", r.ID, name, err)
		}
		failed := len(history) >= need
		for _, h := range history {
			if h.Status != store.BackupStatusFailed {
				failed = false
				break
			}
		}
		v := 0.0
		if failed {
			v = float64(len(history))
		}
		next.LastValue = &v
		next = advanceState(next, r, failed, r.ForDuration, now)
		if !failed {
			return next, "", nil
		}
		return next, fmt.Sprintf("%s: the last %d backup attempts failed", name, len(history)), nil
	}
	return r, "", fmt.Errorf("alerting: rule %q is not a database rule kind", r.ID)
}

func quietDatabaseRule(next Rule) Rule {
	next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
	return next
}

// percentRule advances state for a value-over-threshold rule and reports whether the value is over the line.
func percentRule(next, prev Rule, value, threshold float64, now time.Time) (Rule, bool) {
	next.LastValue = &value
	over := value >= threshold
	return advanceState(next, prev, over, prev.ForDuration, now), over
}

func thresholdOr(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func newestSuccessAge(history []store.BackupHistory, now time.Time) (time.Duration, bool) {
	var newest time.Time
	for _, h := range history {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		t, err := time.Parse(time.RFC3339, h.FinishedAt)
		if err != nil {
			continue
		}
		if t.After(newest) {
			newest = t
		}
	}
	if newest.IsZero() {
		return 0, false
	}
	return now.Sub(newest), true
}
