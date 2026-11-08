package runtimesettings

// KeyAppsEnvApplyOnSet makes env set and import restart the app right away.
const KeyAppsEnvApplyOnSet = "apps.env_apply_on_set"

func init() {
	Register(Definition{
		Key: KeyAppsEnvApplyOnSet, Category: CategoryDeploys, Kind: KindBool, Default: "false",
		Env: "APP_ENV_APPLY_ON_SET", Apply: ApplyLive,
		Title:       "Apply env changes on set",
		Description: "When on, env set and env import restart the app so the new values reach the running container, as if --apply was passed. Pass --no-apply to keep one change pending. Off by default; a change applies to the next command.",
	})
}
