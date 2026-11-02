package runtimesettings

// KeyBackupDefaultRetentionHours is the default backup retention window in hours.
const KeyBackupDefaultRetentionHours = "backup.default_retention_hours"

// BackupRetentionPresets are the hour counts an operator may pick: 1, 2, 7, 30, 60 and 90 days; 0 keeps backups.
var BackupRetentionPresets = []int{0, 24, 48, 168, 720, 1440, 2160}

func init() {
	Register(Definition{
		Key: KeyBackupDefaultRetentionHours, Category: CategoryRetention, Kind: KindInt, Default: "168", AllowedInts: BackupRetentionPresets,
		Env: "APP_BACKUP_DEFAULT_RETENTION_HOURS", Apply: ApplyLive,
		TitleKey: "runtimeSettings:items.backupDefaultRetentionHours.title", DescKey: "runtimeSettings:items.backupDefaultRetentionHours.description",
		Title:       "Delete old scheduled backups after",
		Description: "Hours a scheduled backup is kept when its database or volume has no count or age rule of its own. The newest backup is never removed. 0 keeps them until a rule or a person deletes them.",
	})
}
