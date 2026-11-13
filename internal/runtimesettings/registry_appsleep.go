package runtimesettings

// Limits for requests held while a sleeping app wakes.
const (
	KeyAppSleepMaxWaiters      = "appsleep.max_waiters"
	KeyAppSleepWakeWaitSeconds = "appsleep.wake_wait_seconds" //nolint:gosec // setting key, not a credential
)

func init() {
	Register(Definition{
		Key: KeyAppSleepMaxWaiters, Category: CategoryDeploys, Kind: KindInt, Default: "200",
		Env: "APP_SLEEP_MAX_WAITERS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100000),
		Title:       "Requests held per waking app",
		Description: "How many requests may wait at once for one app to wake. A request past the limit is answered 503 with a Retry-After header instead of being held.",
	})
	Register(Definition{
		Key: KeyAppSleepWakeWaitSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_SLEEP_WAKE_WAIT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "Longest hold while an app wakes",
		Description: "Seconds a held request waits for the app before it gets 503 with Retry-After. 0 uses the window learned from the app's own wake history.",
	})
}
