package runtimesettings

// Registered keys for the plan only git sync of stacks.
const (
	KeyStackSyncTickSecs        = "stacks.sync_tick_seconds"
	KeyStackSyncMinIntervalSecs = "stacks.sync_min_interval_seconds"
	KeyStackSyncMaxRepoMB       = "stacks.sync_max_repo_mb"
	KeyStackSyncFetchTimeout    = "stacks.sync_fetch_timeout_seconds"
	KeyStackSyncMaxFiles        = "stacks.sync_max_files"
	KeyStackSyncMaxFileKB       = "stacks.sync_max_file_kb"
	KeyStackSyncRefreshMinSecs  = "stacks.sync_refresh_min_seconds"
	KeyStackSyncMinFreeMB       = "stacks.sync_min_free_mb"
	KeyStackSyncWebhookMaxKB    = "stacks.sync_webhook_max_body_kb"
)

func init() {
	syncInt := func(key, def, env string, lo, hi int, title, desc string) {
		Register(Definition{
			Key: key, Category: CategoryDeploys, Kind: KindInt, Default: def, Env: env, Apply: ApplyLive,
			CheckValue: intRangeCheck(lo, hi), Title: title, Description: desc,
		})
	}
	syncInt(KeyStackSyncTickSecs, "60", "APP_STACK_SYNC_TICK_SECONDS", 5, 3600, "Stack sync tick",
		"Seconds between wake ups of the stack git sync loop, from 5 to 3600. The loop runs only while a stack has sync enabled. The next tick uses a changed value.")
	syncInt(KeyStackSyncMinIntervalSecs, "60", "APP_STACK_SYNC_MIN_INTERVAL_SECONDS", 10, 86400, "Stack sync interval floor",
		"Shortest allowed gap in seconds between two syncs of one stack, from 10 to 86400. A shorter per stack interval is raised to it. The next tick uses a changed value.")
	syncInt(KeyStackSyncMaxRepoMB, "50", "APP_STACK_SYNC_MAX_REPO_MB", 1, 2048, "Stack sync repository size cap",
		"Largest checkout in MB a stack sync reads, from 1 to 2048. A larger repository fails with sync_limits_exceeded. The next sync uses a changed value.")
	syncInt(KeyStackSyncFetchTimeout, "120", "APP_STACK_SYNC_FETCH_TIMEOUT_SECONDS", 5, 1800, "Stack sync fetch timeout",
		"Seconds a stack sync may spend fetching its repository, from 5 to 1800. The next sync uses a changed value.")
	syncInt(KeyStackSyncMaxFiles, "200", "APP_STACK_SYNC_MAX_FILES", 1, 5000, "Stack sync file cap",
		"Most documents a stack sync reads under its path, from 1 to 5000. More fail with sync_limits_exceeded. The next sync uses a changed value.")
	syncInt(KeyStackSyncMaxFileKB, "1024", "APP_STACK_SYNC_MAX_FILE_KB", 1, 16384, "Stack sync file size cap",
		"Largest single document in KB a stack sync reads, from 1 to 16384. A larger file fails with sync_limits_exceeded. The next sync uses a changed value.")
	syncInt(KeyStackSyncRefreshMinSecs, "30", "APP_STACK_SYNC_REFRESH_MIN_SECONDS", 1, 3600, "Stack sync refresh cooldown",
		"Seconds a stack must wait between two manual sync refreshes, from 1 to 3600. The next refresh uses a changed value.")
	syncInt(KeyStackSyncMinFreeMB, "2048", "APP_STACK_SYNC_MIN_FREE_MB", 0, 1048576, "Stack sync free disk floor",
		"Free disk in MB a stack sync needs before it fetches, from 0 (no check) to 1048576. Below it the sync fails with sync_disk_low and fetches nothing. The next sync uses a changed value.")
	syncInt(KeyStackSyncWebhookMaxKB, "64", "APP_STACK_SYNC_WEBHOOK_MAX_BODY_KB", 1, 25600, "Stack sync webhook body cap",
		"Largest push webhook body in KB the stack sync webhook reads, from 1 to 25600. A larger body is refused with 413 before it is read. The next delivery uses a changed value.")
}
