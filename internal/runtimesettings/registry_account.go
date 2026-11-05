package runtimesettings

// KeyAccountPreferencesMaxBytes caps one saved per-user preference document.
const KeyAccountPreferencesMaxBytes = "account.preferences_max_bytes"

func init() {
	registerLimit(Definition{
		Key: KeyAccountPreferencesMaxBytes, Category: CategoryRetention, Default: "8192", Env: "APP_ACCOUNT_PREFERENCES_MAX_BYTES", Apply: ApplyLive,
		Title:       "Account: saved preference size (bytes)",
		Description: "Largest saved personal preference document (such as appearance synced to your account), from 256 to 65536 bytes. A bigger save is refused with HTTP 413. The next save uses a changed value.",
	}, 256, 65536)
}
