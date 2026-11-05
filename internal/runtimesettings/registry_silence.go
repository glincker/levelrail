package runtimesettings

// KeySecurityProtectedSilenceMaxDays caps how long a root admin may mute a security-integrity issue.
const KeySecurityProtectedSilenceMaxDays = "security.protected_silence_max_days"

// KeySecurityDockerGuardMaxItems caps how many per-rule Docker guard items the attention feed lists.
const KeySecurityDockerGuardMaxItems = "security.docker_guard_max_items"

func init() {
	Register(Definition{
		Key: KeySecurityDockerGuardMaxItems, Category: CategorySecurity, Kind: KindInt, Default: "5",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, 50),
		Title:       "Docker guard items listed",
		Description: "Most per-rule Docker guard findings the issue list shows at once, from 1 to 50. Each high-risk rule is its own item so it can be handled on its own; rules beyond the limit are counted in the last item.",
	})
	// No env seed: a runtime setting only, applied live on the next silence.
	Register(Definition{
		Key: KeySecurityProtectedSilenceMaxDays, Category: CategorySecurity, Kind: KindInt, Default: "30",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 3650),
		Title:       "Longest silence for a security issue",
		Description: "Most days a root admin may silence a security-integrity issue such as the Docker guard, from 0 to 3650. 0 allows silencing forever. Existing silences keep their expiry.",
	})
}
