package runtimesettings

// KeyOnboardingRestoreReminderDays is how many days after onboarding starts the dashboard nudges a restore test.
const KeyOnboardingRestoreReminderDays = "onboarding.restore_reminder_days"

const restoreReminderCeiling = 365

func init() {
	registerLimit(Definition{
		Key: KeyOnboardingRestoreReminderDays, Category: CategoryExperimental, Default: "7",
		Env: "APP_ONBOARDING_RESTORE_REMINDER_DAYS", Apply: ApplyLive,
		Title:       "Onboarding: restore test reminder",
		Description: "Days after a user starts onboarding before the dashboard reminds them to test a restore, when a backup exists and no restore was ever verified. 0 turns the reminder off.",
	}, 0, restoreReminderCeiling)
}
