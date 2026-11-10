package runtimesettings

// Registered keys bounding git deploy trigger tag and branch patterns.
const (
	KeyGitTriggerPatternsMax   = "git.trigger_patterns_max"
	KeyGitTriggerPatternMaxLen = "git.trigger_pattern_max_length"
	KeyGitTriggerRefMaxBytes   = "git.trigger_ref_max_bytes"
	KeyGitTriggerListMaxBytes  = "git.trigger_pattern_list_max_bytes"
)

func init() {
	registerLimit(Definition{
		Key: KeyGitTriggerPatternsMax, Category: CategoryDeploys, Default: "32", Env: "APP_GIT_TRIGGER_PATTERNS_MAX", Apply: ApplyLive,
		Title:       "Trigger patterns per list",
		Description: "Most tag or branch patterns one git source may set. From 1 to 256. Saved lists over a lowered value keep working until edited.",
	}, 1, 256)
	registerLimit(Definition{
		Key: KeyGitTriggerPatternMaxLen, Category: CategoryDeploys, Default: "128", Env: "APP_GIT_TRIGGER_PATTERN_MAX_LENGTH", Apply: ApplyLive,
		Title:       "Trigger pattern length",
		Description: "Longest tag or branch pattern in bytes. From 8 to 1024. Checked when the pattern is saved.",
	}, 8, 1024)
	registerLimit(Definition{
		Key: KeyGitTriggerRefMaxBytes, Category: CategoryDeploys, Default: "1024", Env: "APP_GIT_TRIGGER_REF_MAX_BYTES", Apply: ApplyLive,
		Title:       "Webhook ref length",
		Description: "Longest pushed ref a trigger pattern is matched against; a longer ref never deploys. From 64 to 4096.",
	}, 64, 4096)
	registerLimit(Definition{
		Key: KeyGitTriggerListMaxBytes, Category: CategoryDeploys, Default: "1024", Env: "APP_GIT_TRIGGER_PATTERN_LIST_MAX_BYTES", Apply: ApplyLive,
		Title:       "Trigger pattern list size",
		Description: "Most bytes all tag or branch patterns of one list may total, which bounds the matching cost of each push. From 64 to 4096. Checked when the list is saved.",
	}, 64, 4096)
}
