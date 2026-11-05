package runtimesettings

// KeySecurityProtectedSilenceMaxDays caps how long a root admin may mute a security-integrity issue.
const KeySecurityProtectedSilenceMaxDays = "security.protected_silence_max_days"

func init() {
	// No env seed: a runtime setting only, applied live on the next silence.
	Register(Definition{
		Key: KeySecurityProtectedSilenceMaxDays, Category: CategorySecurity, Kind: KindInt, Default: "30",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 3650),
		Title:       "Longest silence for a security issue",
		Description: "Most days a root admin may silence a security-integrity issue such as the Docker guard, from 0 to 3650. 0 allows silencing forever. Existing silences keep their expiry.",
	})
}
