package runtimesettings

import "time"

// Keys for the database health alert rules.
const (
	KeyAlertsDatabaseCrashLoopEnabled    = "alerts.database.crash_loop_enabled"
	KeyAlertsDatabaseDiskEnabled         = "alerts.database.disk_growth_enabled"
	KeyAlertsDatabaseDiskWarnDays        = "alerts.database.disk_growth_warn_days"
	KeyAlertsDatabaseDiskCriticalDays    = "alerts.database.disk_growth_critical_days"
	KeyAlertsDatabaseDiskFor             = "alerts.database.disk_growth_for"
	KeyAlertsDatabaseConnectionsEnabled  = "alerts.database.connections_enabled"
	KeyAlertsDatabaseConnectionsPercent  = "alerts.database.connections_percent"
	KeyAlertsDatabaseConnectionsFor      = "alerts.database.connections_for"
	KeyAlertsDatabaseBackupEnabled       = "alerts.database.backup_missing_enabled"
	KeyAlertsDatabaseBackupWindow        = "alerts.database.backup_missing_window"
	KeyAlertsDatabaseBackupUnprotectedMB = "alerts.database.backup_unprotected_mb"
)

func init() {
	live := func(key string, kind Kind, def, title, desc string, check func(string) error) {
		Register(Definition{Key: key, Category: CategoryObservability, Kind: kind, Default: def, Apply: ApplyLive, CheckValue: check, Title: title, Description: desc})
	}
	live(KeyAlertsDatabaseCrashLoopEnabled, KindBool, "true", "Alert when a database crash loops",
		"Fires database_crash_loop, critical, while a database container keeps restarting while it starts.", nil)
	live(KeyAlertsDatabaseDiskEnabled, KindBool, "true", "Alert when a database disk will fill",
		"Fires database_disk_growth from the last 7 days of data growth, projected to the size of the data volume.", nil)
	live(KeyAlertsDatabaseDiskWarnDays, KindInt, "14", "Disk full warning, days left",
		"Warn when the forecast says the data volume fills within this many days.", intRangeCheck(1, 365))
	live(KeyAlertsDatabaseDiskCriticalDays, KindInt, "3", "Disk full critical, days left",
		"Raise a critical alert when the forecast says the data volume fills within this many days.", intRangeCheck(1, 365))
	live(KeyAlertsDatabaseDiskFor, KindDuration, "30m", "Disk forecast must hold for",
		"How long the forecast must stay under the limit before the alert fires.", durationRangeCheck(0, 24*time.Hour))
	live(KeyAlertsDatabaseConnectionsEnabled, KindBool, "true", "Alert when a database runs out of connections",
		"Fires database_connections_saturated while connections in use stay over the percent below.", nil)
	live(KeyAlertsDatabaseConnectionsPercent, KindInt, "90", "Connections saturated at percent",
		"Percent of max connections in use that counts as saturated.", intRangeCheck(1, 100))
	live(KeyAlertsDatabaseConnectionsFor, KindDuration, "5m", "Connections must stay saturated for",
		"How long connection use must stay over the percent before the alert fires.", durationRangeCheck(0, 24*time.Hour))
	live(KeyAlertsDatabaseBackupEnabled, KindBool, "true", "Alert when a database has no recent backup",
		"Fires database_backup_missing when a database with backup storage has no successful backup in the window, or warns when a database with real data has no backup storage at all.", nil)
	live(KeyAlertsDatabaseBackupWindow, KindDuration, "26h", "Longest time without a successful backup",
		"A database with backup storage that goes longer than this without a successful backup raises the alert.", durationRangeCheck(time.Hour, 90*24*time.Hour))
	live(KeyAlertsDatabaseBackupUnprotectedMB, KindInt, "512", "Warn about unprotected data over (MB)",
		"A database with no backup storage and at least this much data raises a warning. 0 turns the warning off.", intRangeCheck(0, 10485760))
}
