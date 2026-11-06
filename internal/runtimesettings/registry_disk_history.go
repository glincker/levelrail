package runtimesettings

import "time"

// Disk history setting keys.
const (
	KeyDiskHistoryRetentionDays  = "disk.history_retention_days"
	KeyDiskHistorySampleInterval = "disk.history_sample_interval"
	KeyDiskForecastWindowDays    = "disk.forecast_window_days"
	KeyDiskForecastMinDays       = "disk.forecast_min_days"
	DiskHistoryMinSampleInterval = time.Hour
	diskHistoryMaxSampleInterval = 7 * 24 * time.Hour
	diskHistoryMinRetentionDays  = 7
	diskHistoryMaxRetentionDays  = 730
	diskForecastMinWindowDays    = 7
	diskForecastMaxWindowDays    = 365
	diskForecastMinMinDays       = 2
	diskForecastMaxMinDays       = 30
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
	Register(Definition{
		Key: KeyDiskForecastWindowDays, Category: CategoryObservability, Kind: KindInt, Default: "30",
		Env: "APP_DISK_FORECAST_WINDOW_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(diskForecastMinWindowDays, diskForecastMaxWindowDays),
		Title:       "Disk forecast window",
		Description: "Days of recent samples the days-until-full forecast fits, from 7 to 365; a longer window smooths spikes and reacts slower. The next history request uses a changed value.",
	})
	Register(Definition{
		Key: KeyDiskForecastMinDays, Category: CategoryObservability, Kind: KindInt, Default: "7",
		Env: "APP_DISK_FORECAST_MIN_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(diskForecastMinMinDays, diskForecastMaxMinDays),
		Title:       "Disk forecast minimum days",
		Description: "Distinct days of samples required before a days-until-full number is shown, from 2 to 30, so the estimate is never a guess from a few points.",
	})
}
