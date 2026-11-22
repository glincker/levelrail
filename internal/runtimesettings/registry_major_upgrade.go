package runtimesettings

// KeyMajorUpgradeRestorePointMaxAgeHours is how old the newest successful backup may be
// before the major upgrade check answers no-go.
const KeyMajorUpgradeRestorePointMaxAgeHours = "databases.major_upgrade_restore_point_max_age_hours"

func init() {
	registerLimitInt(KeyMajorUpgradeRestorePointMaxAgeHours, "APP_MAJOR_UPGRADE_RESTORE_POINT_MAX_AGE_HOURS", "24",
		"Major upgrade: oldest acceptable restore point",
		"Hours old the newest successful backup of a database may be before the major upgrade check answers no-go, from 1 to 8760.", 1, 8760)
}
