package runtimesettings

// Keys for the backup size anomaly alert.
const (
	KeyBackupSizeAnomalyPercent  = "backup.size_anomaly_percent"
	KeyBackupSizeAnomalyBaseline = "backup.size_anomaly_baseline"
)

func init() {
	Register(Definition{
		Key: KeyBackupSizeAnomalyPercent, Category: CategoryRetention, Kind: KindInt, Default: "25",
		Env: "APP_BACKUP_SIZE_ANOMALY_PERCENT", Apply: ApplyLive, CheckValue: intRangeCheck(0, 100),
		Title:       "Flag a backup smaller than this percent of usual",
		Description: "A backup that succeeded but is smaller than this percent of the typical earlier size raises a backup_size_anomaly alert, which catches empty dumps and truncated volumes. 0 turns the check off.",
	})
	Register(Definition{
		Key: KeyBackupSizeAnomalyBaseline, Category: CategoryRetention, Kind: KindInt, Default: "3",
		Env: "APP_BACKUP_SIZE_ANOMALY_BASELINE", Apply: ApplyLive, CheckValue: intRangeCheck(1, 30),
		Title:       "Earlier backups used for the usual size",
		Description: "How many earlier successful backups of the same database or volume set its usual size for the size anomaly alert. A resource needs more backups than this before it is checked.",
	})
}
