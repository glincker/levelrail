package runtimesettings

// Registered caps for saved list views, so one user cannot fill the store or the menu.
const (
	KeySavedViewNameMaxChars  = "views.name_max_chars"
	KeySavedViewFilterMaxKeys = "views.filter_max_keys"
	KeySavedViewValueMaxBytes = "views.filter_value_max_bytes"
	KeySavedViewMaxPerUser    = "views.max_per_user"
)

func init() {
	registerLimit(Definition{
		Key: KeySavedViewNameMaxChars, Category: CategoryDeploys, Default: "64", Env: "APP_SAVED_VIEW_NAME_MAX_CHARS", Apply: ApplyLive,
		Title:       "Saved view name length",
		Description: "Most characters in the name of a saved list view. From 1 to 200.",
	}, 1, 200)
	registerLimit(Definition{
		Key: KeySavedViewFilterMaxKeys, Category: CategoryDeploys, Default: "12", Env: "APP_SAVED_VIEW_FILTER_MAX_KEYS", Apply: ApplyLive,
		Title:       "Saved view filter keys",
		Description: "Most filter fields one saved list view may store. From 1 to 32.",
	}, 1, 32)
	registerLimit(Definition{
		Key: KeySavedViewValueMaxBytes, Category: CategoryDeploys, Default: "512", Env: "APP_SAVED_VIEW_FILTER_VALUE_MAX_BYTES", Apply: ApplyLive,
		Title:       "Saved view filter value size",
		Description: "Most bytes in one filter value of a saved list view. From 16 to 4096.",
	}, 16, 4096)
	registerLimit(Definition{
		Key: KeySavedViewMaxPerUser, Category: CategoryDeploys, Default: "100", Env: "APP_SAVED_VIEW_MAX_PER_USER", Apply: ApplyLive,
		Title:       "Saved views per user",
		Description: "Most saved list views one user or token may keep. From 1 to 1000. Existing views are kept when the value is lowered.",
	}, 1, 1000)
}
