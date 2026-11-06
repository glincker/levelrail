package runtimesettings

// KeySecurityDockerGuardMaxItems caps how many per-rule Docker guard items the attention feed lists.
const KeySecurityDockerGuardMaxItems = "security.docker_guard_max_items"

func init() {
	Register(Definition{
		Key: KeySecurityDockerGuardMaxItems, Category: CategorySecurity, Kind: KindInt, Default: "5",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, 50),
		Title:       "Docker guard items listed",
		Description: "Most per-rule Docker guard findings the issue list shows at once, from 1 to 50. Each high-risk rule is its own item so it can be handled on its own; rules beyond the limit are counted in the last item.",
	})
}
