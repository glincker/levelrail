package runtimesettings

// Provider discovery and spend read settings. Env values only seed defaults.
const (
	KeyProvisionResourcesCacheTTL    = "provision.resources.cache_ttl_seconds"
	KeyProvisionResourcesRefreshGap  = "provision.resources.refresh_min_interval_seconds"
	KeyProvisionSpendCacheTTL        = "provision.spend.cache_ttl_seconds"
	KeyProvisionSpendRefreshMinInter = "provision.spend.refresh_min_interval_seconds"
)

func init() {
	cat := CategoryExperimental
	Register(Definition{
		Key: KeyProvisionResourcesCacheTTL, Category: cat, Kind: KindInt, Default: "60", Env: "APP_PROVISION_RESOURCES_CACHE_TTL_SECONDS", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 86400),
		Title:      "Provider discovery: cache time (seconds)", Description: "How long the list of machines in a provider account is served from memory before the provider is asked again.",
	})
	Register(Definition{
		Key: KeyProvisionResourcesRefreshGap, Category: cat, Kind: KindInt, Default: "15", Env: "APP_PROVISION_RESOURCES_REFRESH_MIN_INTERVAL_SECONDS", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 3600),
		Title:      "Provider discovery: forced refresh cooldown (seconds)", Description: "Shortest gap between two forced refreshes of one provider's machine list. A faster request gets a 429 with Retry-After.",
	})
	Register(Definition{
		Key: KeyProvisionSpendCacheTTL, Category: cat, Kind: KindInt, Default: "3600", Env: "APP_PROVISION_SPEND_CACHE_TTL_SECONDS", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 604800),
		Title:      "Provider spend: cache time (seconds)", Description: "How long a provider spend report is served from memory. Provider balances are hours behind real use, so a long cache loses little.",
	})
	Register(Definition{
		Key: KeyProvisionSpendRefreshMinInter, Category: cat, Kind: KindInt, Default: "300", Env: "APP_PROVISION_SPEND_REFRESH_MIN_INTERVAL_SECONDS", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 86400),
		Title:      "Provider spend: forced refresh cooldown (seconds)", Description: "Shortest gap between two forced refreshes of one provider's spend report. A faster request gets a 429 with Retry-After.",
	})
}
