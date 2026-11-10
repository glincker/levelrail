package runtimesettings

// KeyBuildSettingValueMaxBytes caps each per-app build setting value.
const KeyBuildSettingValueMaxBytes = "builds.setting_value_max_bytes"

func init() {
	registerLimit(Definition{
		Key: KeyBuildSettingValueMaxBytes, Category: CategoryDeploys, Default: "128", Env: "APP_BUILD_SETTING_VALUE_MAX_BYTES", Apply: ApplyLive,
		Title:       "Build setting length",
		Description: "Longest Dockerfile path, target stage or provider an app's build settings may store. From 16 to 1024. Checked when the settings are saved.",
	}, 16, 1024)
}
