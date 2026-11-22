package runtimesettings

// KeyBulkMaxPauseSeconds caps the pause between waves of a staggered bulk action.
const KeyBulkMaxPauseSeconds = "bulk.max_pause_seconds"

// Limits for POST /api/v1/apps/bulk and the resource console bulk tag action.
const (
	KeyBulkMaxApps      = "bulk.max_apps"
	KeyBulkConcurrency  = "bulk.concurrency"
	KeyBulkConfirmAbove = "bulk.confirm_above"
	KeyBulkMaxResources = "bulk.max_resources"
)

func init() {
	Register(Definition{
		Key: KeyBulkMaxPauseSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "300",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "Bulk actions: longest pause between waves",
		Description: "Upper limit in seconds for the pause between waves of a staggered bulk action, from 0 to 3600. A longer requested pause is shortened to this value. 0 disables pauses.",
	})
	registerLimit(Definition{
		Key: KeyBulkMaxApps, Category: CategoryDeploys, Default: "200", Env: "APP_BULK_MAX_APPS", Apply: ApplyLive,
		Title:       "Bulk actions: most apps per request",
		Description: "Most apps one bulk action (POST /api/v1/apps/bulk) may target; a bigger selection is refused with HTTP 400 and nothing runs. From 1 to 2000. The next request uses a changed value.",
	}, 1, 2000)
	registerLimit(Definition{
		Key: KeyBulkConcurrency, Category: CategoryDeploys, Default: "4", Env: "APP_BULK_CONCURRENCY", Apply: ApplyLive,
		Title:       "Bulk actions: apps run at once",
		Description: "How many apps one bulk action works on at the same time, within a wave. From 1 to 32. The next wave uses a changed value.",
	}, 1, 32)
	registerLimit(Definition{
		Key: KeyBulkConfirmAbove, Category: CategoryDeploys, Default: "10", Env: "APP_BULK_CONFIRM_ABOVE", Apply: ApplyLive,
		Title:       "Bulk actions: confirm above this many apps",
		Description: "A bulk redeploy, restart or stop over more apps than this needs confirm_count equal to the number of targets. From 1 to 2000. The next request uses a changed value.",
	}, 1, 2000)
	registerLimit(Definition{
		Key: KeyBulkMaxResources, Category: CategoryDeploys, Default: "500", Env: "APP_BULK_MAX_RESOURCES", Apply: ApplyLive,
		Title:       "Bulk actions: most resources per tag request",
		Description: "Most resources one bulk tag request in the resource console may carry. From 1 to 5000. The next request uses a changed value.",
	}, 1, 5000)
}
