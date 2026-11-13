package runtimesettings

// Keys for the error tracking event-ID dedup window.
const (
	KeyErrorsDedupHours = "errors.dedup_hours"
	KeyErrorsDedupMax   = "errors.dedup_max"
)

func init() {
	registerLimit(Definition{
		Key: KeyErrorsDedupHours, Category: CategoryObservability, Default: "168", Env: "APP_ERROR_DEDUP_HOURS", Apply: ApplyLive,
		Title:       "Error events: dedup window",
		Description: "Hours an error event ID is remembered so a retried envelope is counted once. From 1 to 8760.",
	}, 1, 8760)
	registerLimit(Definition{
		Key: KeyErrorsDedupMax, Category: CategoryObservability, Default: "50000", Env: "APP_ERROR_DEDUP_MAX", Apply: ApplyLive,
		Title:       "Error events: dedup memory",
		Description: "Most event IDs remembered per app for dedup. From 1000 to 5000000. The oldest are dropped first.",
	}, 1000, 5_000_000)
}
