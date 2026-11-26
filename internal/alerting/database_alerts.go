package alerting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/forecast"
)

// Database health rule kinds. Each fans out over every managed database and reads its thresholds from live settings.
const (
	KindDatabaseCrashLoop            Kind = "database_crash_loop"
	KindDatabaseDiskGrowth           Kind = "database_disk_growth"
	KindDatabaseConnectionsSaturated Kind = "database_connections_saturated"
	KindDatabaseBackupMissing        Kind = "database_backup_missing"
)

// IsDatabaseHealthKind reports whether k is one of the metric-driven database health kinds.
func IsDatabaseHealthKind(k Kind) bool {
	switch k {
	case KindDatabaseCrashLoop, KindDatabaseDiskGrowth, KindDatabaseConnectionsSaturated, KindDatabaseBackupMissing:
		return true
	}
	return false
}

// clearMargin keeps a firing rule firing until the value is clearly back inside its limit, so a reading that
// hovers on the line does not flap.
const (
	connClearPoints = 5.0
	diskClearFactor = 1.25
)

// DatabaseAlertPolicy is the live threshold set; the engine reads it on every tick.
type DatabaseAlertPolicy struct {
	CrashLoopEnabled bool

	DiskGrowthEnabled bool
	DiskWarnDays      float64
	DiskCriticalDays  float64
	DiskFor           time.Duration

	ConnectionsEnabled bool
	ConnectionsPercent float64
	ConnectionsFor     time.Duration

	BackupEnabled bool
	// BackupWindow is the longest a database with a backup target may go without a successful backup.
	BackupWindow time.Duration
	// BackupUnprotectedBytes is the size past which a database with no backup target gets a warning; 0 turns it off.
	BackupUnprotectedBytes int64
}

// DatabaseAlertInput is everything one database contributes to the four rules. A zero field means "not measured",
// so a missing metric never breaches.
type DatabaseAlertInput struct {
	Name string

	CrashLooping bool
	CrashMessage string

	ConnTotal float64
	ConnMax   float64

	// DiskPoints are used-bytes samples over the last 7 days and DiskTotalBytes the data volume size.
	DiskPoints     []forecast.Point
	DiskTotalBytes float64

	BackupTarget   bool
	BackupSchedule string
	// LastBackupAt is the newest successful backup; zero means none yet.
	LastBackupAt time.Time
	// CreatedAt bounds the window for a database that never had a backup.
	CreatedAt time.Time
	SizeBytes float64
}

// DatabaseAlertSource lists one input per running, non-suspended managed database.
type DatabaseAlertSource interface {
	DatabaseAlertInputs(ctx context.Context) ([]DatabaseAlertInput, error)
}

// DatabaseAlertPolicySource reads the live policy.
type DatabaseAlertPolicySource func(ctx context.Context) DatabaseAlertPolicy

// DatabaseBreach is one database over one rule's line.
type DatabaseBreach struct {
	Database string
	Value    float64
	Notice   string
}

// databasePath is the dashboard deep link for a database page section.
func databasePath(name, page string) string {
	return "/databases/" + name + "/" + page
}

// DatabaseBreaches returns who is over the line for one rule kind. wasFiring says the rule already fires, which widens the line so the rule only clears once the value is clearly back inside it.
func DatabaseBreaches(kind Kind, severity string, in []DatabaseAlertInput, p DatabaseAlertPolicy, wasFiring bool, now time.Time) []DatabaseBreach {
	var out []DatabaseBreach
	for _, d := range in {
		if b, ok := databaseBreach(kind, severity, d, p, wasFiring, now); ok {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Database < out[j].Database })
	return out
}

func databaseBreach(kind Kind, severity string, d DatabaseAlertInput, p DatabaseAlertPolicy, wasFiring bool, now time.Time) (DatabaseBreach, bool) {
	switch kind {
	case KindDatabaseCrashLoop:
		if !p.CrashLoopEnabled || !d.CrashLooping {
			return DatabaseBreach{}, false
		}
		msg := d.CrashMessage
		if msg != "" {
			msg = " Last output: " + msg
		}
		return DatabaseBreach{Database: d.Name, Value: 1, Notice: fmt.Sprintf(
			"%s keeps crashing on start.%s What to do: open the database logs to see why, then fix the setting or restore a backup. %s",
			d.Name, msg, databasePath(d.Name, "metrics"))}, true
	case KindDatabaseConnectionsSaturated:
		if !p.ConnectionsEnabled || d.ConnMax <= 0 || p.ConnectionsPercent <= 0 {
			return DatabaseBreach{}, false
		}
		pct := d.ConnTotal / d.ConnMax * 100
		line := p.ConnectionsPercent
		if wasFiring {
			line -= connClearPoints
		}
		if pct < line {
			return DatabaseBreach{}, false
		}
		return DatabaseBreach{Database: d.Name, Value: pct, Notice: fmt.Sprintf(
			"%s is using %.0f of %.0f connections (%.0f%%, alert at %.0f%%). What to do: close idle clients, enable the connection pooler or raise max connections. %s",
			d.Name, d.ConnTotal, d.ConnMax, pct, p.ConnectionsPercent, databasePath(d.Name, "metrics"))}, true
	case KindDatabaseDiskGrowth:
		if !p.DiskGrowthEnabled {
			return DatabaseBreach{}, false
		}
		proj := forecast.Project(d.DiskPoints, d.DiskTotalBytes, now)
		if proj == nil {
			return DatabaseBreach{}, false
		}
		limit := p.DiskWarnDays
		if severity == SeverityCritical {
			limit = p.DiskCriticalDays
		}
		if limit <= 0 {
			return DatabaseBreach{}, false
		}
		if wasFiring {
			limit *= diskClearFactor
		}
		if proj.DaysUntilFull > limit {
			return DatabaseBreach{}, false
		}
		return DatabaseBreach{Database: d.Name, Value: proj.DaysUntilFull, Notice: fmt.Sprintf(
			"%s data volume is forecast to fill in %.1f days at the current growth of %s per day. What to do: free space, prune old data or move to a bigger volume. %s",
			d.Name, proj.DaysUntilFull, humanBytes(proj.SlopeBytesPerDay), databasePath(d.Name, "metrics"))}, true
	case KindDatabaseBackupMissing:
		if !p.BackupEnabled {
			return DatabaseBreach{}, false
		}
		if !d.BackupTarget {
			if p.BackupUnprotectedBytes <= 0 || d.SizeBytes < float64(p.BackupUnprotectedBytes) {
				return DatabaseBreach{}, false
			}
			return DatabaseBreach{Database: d.Name, Value: d.SizeBytes, Notice: fmt.Sprintf(
				"%s holds %s and has no backup storage configured. What to do: add a backup target and a schedule. %s",
				d.Name, humanBytes(d.SizeBytes), databasePath(d.Name, "settings")+"?section=backups")}, true
		}
		if p.BackupWindow <= 0 || d.BackupSchedule == "" {
			return DatabaseBreach{}, false
		}
		since := d.LastBackupAt
		if since.IsZero() {
			since = d.CreatedAt
		}
		if since.IsZero() {
			return DatabaseBreach{}, false
		}
		age := now.Sub(since)
		window := p.BackupWindow
		if wasFiring {
			window = window * 9 / 10
		}
		if age <= window {
			return DatabaseBreach{}, false
		}
		what := "the last successful backup was"
		if d.LastBackupAt.IsZero() {
			what = "no successful backup since it was created,"
		}
		return DatabaseBreach{Database: d.Name, Value: age.Hours(), Notice: fmt.Sprintf(
			"%s has no successful backup inside %.0fh (%s %.0fh ago). What to do: run a backup now and check the backup target. %s",
			d.Name, p.BackupWindow.Hours(), what, age.Hours(), databasePath(d.Name, "settings")+"?section=backups")}, true
	}
	return DatabaseBreach{}, false
}

// EvaluateDatabaseAlert runs one database health rule over every database. It fires while any database is over
// the line for the policy's for-duration and resolves once all are clearly back inside it.
func EvaluateDatabaseAlert(ctx context.Context, src DatabaseAlertSource, policy DatabaseAlertPolicy, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	inputs, err := src.DatabaseAlertInputs(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read database inputs: %w", r.ID, err)
	}
	breaches := DatabaseBreaches(r.Kind, r.Severity, inputs, policy, r.Firing, now)
	v := float64(len(breaches))
	next.LastValue = &v
	next = advanceState(next, r, len(breaches) > 0, databaseRuleFor(r.Kind, policy), now)
	if len(breaches) == 0 {
		return next, "", nil
	}
	notes := make([]string, len(breaches))
	for i, b := range breaches {
		notes[i] = b.Notice
	}
	return next, strings.Join(notes, "; "), nil
}

func databaseRuleFor(kind Kind, p DatabaseAlertPolicy) time.Duration {
	switch kind {
	case KindDatabaseDiskGrowth:
		return p.DiskFor
	case KindDatabaseConnectionsSaturated:
		return p.ConnectionsFor
	}
	return 0
}

// DatabaseAlertingNames lists the names of databases currently breaching any health rule, for the doctor row.
func DatabaseAlertingNames(in []DatabaseAlertInput, p DatabaseAlertPolicy, now time.Time) []string {
	seen := map[string]bool{}
	var names []string
	for _, k := range []struct {
		kind Kind
		sev  string
	}{{KindDatabaseCrashLoop, SeverityCritical}, {KindDatabaseConnectionsSaturated, SeverityWarning}, {KindDatabaseDiskGrowth, SeverityWarning}, {KindDatabaseBackupMissing, SeverityWarning}} {
		for _, b := range DatabaseBreaches(k.kind, k.sev, in, p, false, now) {
			if !seen[b.Database] {
				seen[b.Database] = true
				names = append(names, b.Database)
			}
		}
	}
	sort.Strings(names)
	return names
}

func humanBytes(v float64) string {
	const unit = 1024.0
	if v < unit {
		return fmt.Sprintf("%.0f B", v)
	}
	div, exp := unit, 0
	for n := v / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", v/div, "KMGTP"[exp])
}
