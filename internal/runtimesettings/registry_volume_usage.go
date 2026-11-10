package runtimesettings

import "time"

// Registered keys for volume usage alerts and the volume size history.
const (
	KeyVolumesUsageAlertPercent    = "volumes.usage_alert_percent"
	KeyVolumesSizeHistoryRetention = "volumes.size_history_retention"
	minVolumeHistoryRetention      = time.Hour
	maxVolumeHistoryRetention      = 365 * 24 * time.Hour
)

func init() {
	registerLimit(Definition{
		Key: KeyVolumesUsageAlertPercent, Category: CategoryObservability, Default: "90", Env: "APP_VOLUMES_USAGE_ALERT_PERCENT", Apply: ApplyLive,
		Title:       "Volume usage alert threshold",
		Description: "Percent of a volume's capacity at which a volume usage rule fires, from 1 to 100. The rule resolves once usage drops below it. A change applies to the next evaluation.",
	}, 1, 100)
	Register(Definition{
		Key: KeyVolumesSizeHistoryRetention, Category: CategoryObservability, Kind: KindDuration, Default: "168h",
		Env: "APP_VOLUMES_SIZE_HISTORY_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(minVolumeHistoryRetention, maxVolumeHistoryRetention),
		Title:       "Volume size history retention",
		Description: "How long volume size samples are kept for the size history, from 1h to 8760h. A change applies to the next prune.",
	})
}
