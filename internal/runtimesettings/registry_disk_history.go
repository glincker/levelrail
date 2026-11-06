package runtimesettings

import "time"

// Disk history setting keys.
const (
	KeyDiskHistoryRetentionDays  = "disk.history_retention_days"
	KeyDiskHistorySampleInterval = "disk.history_sample_interval"
	DiskHistoryMinSampleInterval = time.Hour
	diskHistoryMaxSampleInterval = 7 * 24 * time.Hour
	diskHistoryMinRetentionDays  = 7
	diskHistoryMaxRetentionDays  = 730
)

func init() {
	Register(Definition{
		Key: KeyDiskHistoryRetentionDays, Category: CategoryObservability, Kind: KindInt, Default: "90",
		Env: "APP_DISK_HISTORY_RETENTION_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(diskHistoryMinRetentionDays, diskHistoryMaxRetentionDays),
		Title:       "Disk history retention",
		Description: "Days of daily disk samples kept for the storage trend and forecast, from 7 to 730; older samples are deleted on the next sampler tick.",
	})
	Register(Definition{
		Key: KeyDiskHistorySampleInterval, Category: CategoryObservability, Kind: KindDuration, Default: "24h",
		Env: "APP_DISK_HISTORY_SAMPLE_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(DiskHistoryMinSampleInterval, diskHistoryMaxSampleInterval),
		Title:       "Disk history sample interval",
		Description: "How often the control plane records free space and per-category usage for the storage trend, from 1h to 168h; a sample is also taken at boot when the last one is older than this.",
	})
}
