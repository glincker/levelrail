package runtimesettings

// KeyBulkMaxPauseSeconds caps the pause between waves of a staggered bulk action.
const KeyBulkMaxPauseSeconds = "bulk.max_pause_seconds"

func init() {
	Register(Definition{
		Key: KeyBulkMaxPauseSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "300",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "Bulk actions: longest pause between waves",
		Description: "Upper limit in seconds for the pause between waves of a staggered bulk action, from 0 to 3600. A longer requested pause is shortened to this value. 0 disables pauses.",
	})
}
