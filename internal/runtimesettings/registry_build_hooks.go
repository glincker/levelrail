package runtimesettings

// Incoming build hook limits. The env vars only seed the defaults; a saved
// value applies to the next hook call without a restart.
const (
	KeyBuildHookTriggersPerMinute = "deploy.build_hook_triggers_per_minute"
	KeyBuildHookMaxCIDRs          = "deploy.build_hook_max_cidrs"
)

func init() {
	registerLimit(Definition{
		Key: KeyBuildHookTriggersPerMinute, Category: CategoryDeploys, Default: "6", Env: "APP_BUILD_HOOK_TRIGGERS_PER_MINUTE", Apply: ApplyLive,
		Title:       "Build hook calls per minute",
		Description: "Most calls one build hook accepts per minute, from 1 to 600. Calls refused by the source allowlist use a separate bucket of the same size, so a blocked caller is throttled without using up the allowed budget. Over the limit the hook answers 429. The next call uses a changed value.",
	}, 1, 600)
	registerLimit(Definition{
		Key: KeyBuildHookMaxCIDRs, Category: CategoryDeploys, Default: "32", Env: "APP_BUILD_HOOK_MAX_CIDRS", Apply: ApplyLive,
		Title:       "Build hook allowlist size",
		Description: "Most source address ranges (CIDRs) one build hook's allowlist may hold, from 1 to 256. Lowering it keeps longer lists already saved; the next edit must fit.",
	}, 1, 256)
}
