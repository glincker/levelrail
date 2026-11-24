package runtimesettings

// Keys for env revision history.
const (
	KeyEnvRevisionsKeep  = "env.revisions_keep"
	KeyAppsUndoWindowDay = "apps.undo_window_days"
)

func init() {
	Register(Definition{
		Key: KeyEnvRevisionsKeep, Category: CategoryRetention, Kind: KindInt, Default: "20",
		Env: "APP_ENV_REVISIONS_KEEP", Apply: ApplyLive, CheckValue: intRangeCheck(1, 500),
		Title:       "Env revisions kept per app",
		Description: "How many saved env states each app keeps, from 1 to 500. Older ones are pruned on the next env edit. Values are stored encrypted and never shown.",
	})
	Register(Definition{
		Key: KeyAppsUndoWindowDay, Category: CategoryRetention, Kind: KindInt, Default: "7",
		Env: "APP_APPS_UNDO_WINDOW_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 365),
		Title:       "Undo window for app changes",
		Description: "Days an earlier env state stays available to compare, from 1 to 365. The newest revision is always kept.",
	})
}
