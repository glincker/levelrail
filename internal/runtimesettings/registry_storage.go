package runtimesettings

import (
	"fmt"
	"strconv"
	"time"
)

// Disk guard and telemetry storage setting keys.
const (
	KeyDiskLowFreeMB      = "disk.low_free_mb"
	KeyDiskCriticalFreeMB = "disk.critical_free_mb"
	KeyDiskReserveMB      = "disk.reserve_mb"
	KeyMetricsRetention   = "telemetry.metrics_retention"
	KeyLogsRetention      = "telemetry.logs_retention"
	KeyLogBudgetMB        = "telemetry.log_budget_mb"
	KeyMonitorRawRetained = "monitors.raw_retention"
)

// Retention bounds: an hour floor so a typo cannot wipe data, a year ceiling.
const (
	minTelemetryRetention = time.Hour
	maxTelemetryRetention = 365 * 24 * time.Hour
)

func durationRangeCheck(floor, ceil time.Duration) func(string) error {
	return func(canonical string) error {
		d, err := time.ParseDuration(canonical)
		if err != nil || d < floor || d > ceil {
			return fmt.Errorf("value must be a duration between %s and %s", floor, ceil)
		}
		return nil
	}
}

// logBudgetCheck allows 0 (unlimited) or a budget big enough to hold recent lines.
func logBudgetCheck(canonical string) error {
	n, err := strconv.Atoi(canonical)
	if err != nil || n < 0 || (n > 0 && n < 16) || n > 10485760 {
		return fmt.Errorf("value must be 0 (unlimited) or between 16 and 10485760 MiB")
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyDiskLowFreeMB, Category: CategoryObservability, Kind: KindInt, Default: "1024",
		Env: "APP_DISK_LOW_FREE_MB", Apply: ApplyLive, CheckValue: intRangeCheck(64, 1048576),
		Title:       "Low disk threshold",
		Description: "Free space in MiB on the data or Docker volume below which the disk guard raises a low-disk warning and alert (64 to 1048576). Critical is clamped to this value when it is higher.",
	})
	Register(Definition{
		Key: KeyDiskCriticalFreeMB, Category: CategoryObservability, Kind: KindInt, Default: "512",
		Env: "APP_DISK_CRITICAL_FREE_MB", Apply: ApplyLive, CheckValue: intRangeCheck(16, 1048576),
		Title:       "Critical disk threshold",
		Description: "Free space in MiB below which builds, image pulls and backups are refused and the disk reserve is released (16 to 1048576). A value above the low threshold is treated as equal to it.",
	})
	Register(Definition{
		Key: KeyDiskReserveMB, Category: CategoryObservability, Kind: KindInt, Default: "16",
		Env: "APP_DISK_RESERVE_MB", Apply: ApplyLive, CheckValue: intRangeCheck(0, 4096),
		Title:       "Disk reserve size",
		Description: "Size in MiB of the placeholder file kept in the data dir so SQLite can still commit when the disk fills; it is released below the critical threshold (0 to 4096, 0 turns the reserve off).",
	})
	Register(Definition{
		Key: KeyMetricsRetention, Category: CategoryObservability, Kind: KindDuration, Default: "360h",
		Env: "APP_METRICS_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(minTelemetryRetention, maxTelemetryRetention),
		Title:       "Raw metrics retention",
		Description: "How long raw container metric samples are kept before the prune job deletes them, from 1h to 8760h; older data stays available in the coarser rollup tiers.",
	})
	Register(Definition{
		Key: KeyLogsRetention, Category: CategoryObservability, Kind: KindDuration, Default: "360h",
		Env: "APP_LOGS_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(minTelemetryRetention, maxTelemetryRetention),
		Title:       "Log, trace and error retention",
		Description: "How long stored app logs, traces and error events are kept before the hourly sweep deletes them, from 1h to 8760h; the next sweep uses a changed value.",
	})
	Register(Definition{
		Key: KeyLogBudgetMB, Category: CategoryObservability, Kind: KindInt, Default: "0",
		Env: "APP_LOG_BUDGET_MB", Apply: ApplyLive, CheckValue: logBudgetCheck,
		Title:       "Default log budget per app",
		Description: "Stored log size in MiB each app may use before the sweep trims its oldest lines; 0 means unlimited, otherwise 16 to 10485760. A per-app budget overrides it.",
	})
	Register(Definition{
		Key: KeyMonitorRawRetained, Category: CategoryObservability, Kind: KindDuration, Default: "48h",
		Env: "APP_MONITOR_RAW_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(minTelemetryRetention, maxTelemetryRetention),
		Title:       "Monitor raw result retention",
		Description: "How long each uptime monitor check result is kept before the prune job deletes it, from 1h to 8760h; hourly rollups are kept on their own window.",
	})
}
