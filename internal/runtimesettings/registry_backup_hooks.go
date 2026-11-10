package runtimesettings

// Volume backup hook budgets. Hooks run inside the app's own container, so
// both are clamped to keep a hung or chatty hook from holding a backup open.
const (
	KeyBackupHookTimeoutSeconds = "backup.hook_timeout_seconds"
	KeyBackupHookMaxOutputBytes = "backup.hook_max_output_bytes"
)

func init() {
	Register(Definition{
		Key: KeyBackupHookTimeoutSeconds, Category: CategoryRetention, Kind: KindInt, Default: "60",
		Env: "APP_BACKUP_HOOK_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 600),
		Title:       "Backup hook time limit",
		Description: "Seconds a volume backup's pre or post command may run inside the app's container, from 1 to 600, and never above APP_BACKUP_HOOK_MAX_SECONDS. A pre command that runs longer is killed and the backup is not taken; the post command still runs. The next hook uses a changed value.",
	})
	Register(Definition{
		Key: KeyBackupHookMaxOutputBytes, Category: CategoryRetention, Kind: KindInt, Default: "4096",
		Env: "APP_BACKUP_HOOK_MAX_OUTPUT_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(256, 65536),
		Title:       "Backup hook output kept",
		Description: "Bytes of a failing hook's output kept with the backup record, from 256 to 65536, with the command text and known secret shapes masked. The rest is read and discarded. The next hook uses a changed value.",
	})
}
