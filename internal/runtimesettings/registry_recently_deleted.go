package runtimesettings

// KeyRetentionRecentlyDeletedDays is how long a deleted app or database can be restored.
const KeyRetentionRecentlyDeletedDays = "retention.recently_deleted_days"

func init() {
	Register(Definition{
		Key: KeyRetentionRecentlyDeletedDays, Category: CategoryRetention, Kind: KindInt, Default: "7",
		Env: "APP_RETENTION_RECENTLY_DELETED_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 365),
		Title:       "Undo window for deleted apps and databases",
		Description: "Days a deleted app or database keeps its config and a reference to its last backup so it can be restored, from 0 to 365. 0 turns the undo window off. Secrets are never kept. A data volume that was deleted with the app is not recoverable.",
	})
}
