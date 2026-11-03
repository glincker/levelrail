package runtimesettings

// KeyUpgradeVerifyBackup turns on the scratch restore of the pre-upgrade backup.
const KeyUpgradeVerifyBackup = "upgrade.verify_backup"

func init() {
	Register(Definition{
		Key: KeyUpgradeVerifyBackup, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Apply:       ApplyLive,
		Title:       "Verify the pre-upgrade backup",
		Description: "Before an upgrade goes on, restore the fresh control plane backup into a temporary copy, check it and delete the copy. A backup that does not restore refuses the upgrade. The check only reads the backup and never touches the live data. It is skipped with a warning when free disk space is short.",
	})
}
