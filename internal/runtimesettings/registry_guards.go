package runtimesettings

// Registered keys for plan and prune guards, model fetch limits and placement limits.
const (
	KeyIaCPlanValueMax        = "iac.plan_value_max"
	KeyStackPruneMaxPercent   = "stacks.prune_max_percent"
	KeyStackPruneMaxCount     = "stacks.prune_max_count"
	KeyHFTimeoutSeconds       = "models.hf_timeout_seconds"
	KeyHFMaxResponseBytes     = "models.hf_max_response_bytes"
	KeyHFMaxListedFiles       = "models.hf_max_listed_files"
	KeyPlacementAffinityTerms = "placement.affinity_max_terms"
)

func init() {
	registerLimit(Definition{
		Key: KeyIaCPlanValueMax, Category: CategoryDeploys, Default: "120", Env: "APP_IAC_PLAN_VALUE_MAX", Apply: ApplyLive,
		Title:       "Plan value length",
		Description: "Longest before or after value a plan shows, in characters. From 20 to 10000. The next plan uses a changed value.",
	}, 20, 10000)
	registerLimit(Definition{
		Key: KeyStackPruneMaxPercent, Category: CategorySecurity, Default: "50", Env: "APP_STACK_PRUNE_MAX_PERCENT", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Prune guard: share of a stack",
		Description: "A prune removing more than this percent of a stack needs allow_mass_prune. From 0 to 100; 0 means every prune needs the flag.",
	}, 0, 100)
	registerLimit(Definition{
		Key: KeyStackPruneMaxCount, Category: CategorySecurity, Default: "10", Env: "APP_STACK_PRUNE_MAX_COUNT", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Prune guard: item count",
		Description: "A prune removing more items than this needs allow_mass_prune. From 0 to 100000; 0 means every prune needs the flag.",
	}, 0, 100000)
	registerLimit(Definition{
		Key: KeyHFTimeoutSeconds, Category: CategoryNetworking, Default: "10", Env: "APP_HF_TIMEOUT_SECONDS", Apply: ApplyRestart,
		Title:       "Model hub request timeout",
		Description: "Seconds one model hub request may take. From 1 to 300. The client is built at startup, so a change needs a restart.",
	}, 1, 300)
	registerLimit(Definition{
		Key: KeyHFMaxResponseBytes, Category: CategoryNetworking, Default: "16777216", Env: "APP_HF_MAX_RESPONSE_BYTES", Apply: ApplyLive,
		Title:       "Model hub response cap",
		Description: "Largest model hub listing response in bytes, from 1 MiB to 1 GiB. The next fetch uses a changed value.",
	}, bodyLimitFloor, 1<<30)
	registerLimit(Definition{
		Key: KeyHFMaxListedFiles, Category: CategoryNetworking, Default: "100", Env: "APP_HF_MAX_LISTED_FILES", Apply: ApplyLive,
		Title:       "Model preflight listed files",
		Description: "Most files a model preflight lists before it marks the list truncated. From 1 to 10000.",
	}, 1, 10000)
	registerLimit(Definition{
		Key: KeyPlacementAffinityTerms, Category: CategoryDeploys, Default: "8", Env: "APP_PLACEMENT_AFFINITY_MAX_TERMS", Apply: ApplyLive,
		Title:       "Placement affinity terms",
		Description: "Most affinity terms one app may carry. From 1 to 64. Saved rules are kept when the value is lowered.",
	}, 1, 64)
}
