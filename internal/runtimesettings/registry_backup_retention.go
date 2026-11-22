package runtimesettings

import "fmt"

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

// Backup run window and upload speed cap keys.
const (
	KeyBackupRunWindow      = "backup.run_window"
	KeyBackupUploadKBPerSec = "backup.upload_kb_per_sec"
)

// ParseBackupWindow reads "HH:MM-HH:MM" (server local time) into start and end minutes of the day.
// An empty value means no window. A window may wrap midnight, such as 22:00-05:00.
func ParseBackupWindow(v string) (start, end int, ok bool, err error) {
	if v == "" {
		return 0, 0, false, nil
	}
	var sh, sm, eh, em int
	if n, scanErr := fmt.Sscanf(v, "%d:%d-%d:%d", &sh, &sm, &eh, &em); scanErr != nil || n != 4 || len(v) != 11 ||
		sh < 0 || sh > 23 || eh < 0 || eh > 23 || sm < 0 || sm > 59 || em < 0 || em > 59 {
		return 0, 0, false, fmt.Errorf("use HH:MM-HH:MM, for example 22:00-05:00")
	}
	start, end = sh*60+sm, eh*60+em
	if start == end {
		return 0, 0, false, fmt.Errorf("window start and end must differ")
	}
	return start, end, true, nil
}

func init() {
	Register(Definition{
		Key: KeyBackupRunWindow, Category: CategoryRetention, Kind: KindString, Default: "", Apply: ApplyLive,
		CheckValue:  func(v string) error { _, _, _, err := ParseBackupWindow(v); return err },
		Title:       "Run scheduled backups only between",
		Description: "Server local time window such as 22:00-05:00 (it may wrap midnight). A backup that comes due outside it waits until the window opens. Empty runs backups at any time.",
	})
	Register(Definition{
		Key: KeyBackupUploadKBPerSec, Category: CategoryRetention, Kind: KindInt, Default: "0", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 10_000_000),
		Title:       "Backup upload speed limit (KB/s)",
		Description: "Caps how fast one backup upload sends data so a nightly run does not starve apps on a small VPS. 0 means no limit.",
	})
}
