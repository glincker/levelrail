package runtimesettings

// Cleanup run history and pause setting keys.
const (
	KeyCleanupRunsKeep          = "cleanup.runs_keep"
	KeyCleanupMaxFailures       = "cleanup.max_consecutive_failures"
	cleanupRunsKeepMax          = 100000
	cleanupMaxConsecutiveFailMx = 100
)

func init() {
	Register(Definition{
		Key: KeyCleanupRunsKeep, Category: CategoryRetention, Kind: KindInt, Default: "500",
		Env: "APP_CLEANUP_RUNS_KEEP", Apply: ApplyLive, CheckValue: intRangeCheck(10, cleanupRunsKeepMax),
		Title:       "Cleanup runs kept per target",
		Description: "Newest cleanup runs kept for each target, from 10 to 100000. Older runs are removed on the next scheduled pass, on top of the age limit. A change applies to the next pass.",
	})
	Register(Definition{
		Key: KeyCleanupMaxFailures, Category: CategoryRetention, Kind: KindInt, Default: "5",
		Env: "APP_CLEANUP_MAX_CONSECUTIVE_FAILURES", Apply: ApplyLive, CheckValue: intRangeCheck(0, cleanupMaxConsecutiveFailMx),
		Title:       "Cleanup failures before pausing a target",
		Description: "A target whose last this-many real runs all failed is skipped by the scheduler and one alert is sent, from 1 to 100; 0 never pauses. A manual run that succeeds resumes it.",
	})
}
