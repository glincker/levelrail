package runtimesettings

import "time"

// Cleanup failure alert setting keys.
const (
	KeyCleanupFailingWindow = "cleanup.failing_window"
	KeyCleanupFailingFloor  = "cleanup.failing_floor"
	cleanupFailingMinWindow = 15 * time.Minute
	cleanupFailingMaxWindow = 30 * 24 * time.Hour
	cleanupFailingMaxFloor  = 100
)

func init() {
	Register(Definition{
		Key: KeyCleanupFailingWindow, Category: CategoryRetention, Kind: KindDuration, Default: "24h",
		Env: "APP_CLEANUP_FAILING_WINDOW", Apply: ApplyLive, CheckValue: durationRangeCheck(cleanupFailingMinWindow, cleanupFailingMaxWindow),
		Title:       "Cleanup failure look-back",
		Description: "How far back the cleanup_failing alert counts failed cleanup runs, from 15m to 720h. A dry run never counts.",
	})
	Register(Definition{
		Key: KeyCleanupFailingFloor, Category: CategoryRetention, Kind: KindInt, Default: "1",
		Env: "APP_CLEANUP_FAILING_FLOOR", Apply: ApplyLive, CheckValue: intRangeCheck(1, cleanupFailingMaxFloor),
		Title:       "Cleanup failures before alerting",
		Description: "Failed cleanup runs inside the look-back that fire a cleanup_failing rule whose own threshold is 0, from 1 to 100.",
	})
}
