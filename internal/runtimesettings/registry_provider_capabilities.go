package runtimesettings

// Provider capability check settings. Env values only seed defaults.
const (
	KeyProvisionCapCacheTTL      = "provision.capabilities.cache_ttl_seconds"
	KeyProvisionCapRecheckMin    = "provision.capabilities.recheck_interval_minutes"
	KeyProvisionCapRefreshMinGap = "provision.capabilities.refresh_min_interval_seconds"
	KeyProvisionCapProbeParallel = "provision.capabilities.probe_concurrency"
)

func init() {
	cat := CategoryExperimental
	Register(Definition{
		Key: KeyProvisionCapCacheTTL, Category: cat, Kind: KindInt, Default: "900", Env: "APP_PROVISION_CAPABILITIES_CACHE_TTL_SECONDS", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 86400),
		Title:      "Provider permissions: cache time (seconds)", Description: "How long a provider permission check is served from memory before the control plane asks the provider again.",
	})
	Register(Definition{
		Key: KeyProvisionCapRecheckMin, Category: cat, Kind: KindInt, Default: "360", Env: "APP_PROVISION_CAPABILITIES_RECHECK_INTERVAL_MINUTES", Apply: ApplyLive,
		CheckValue: intRangeCheck(0, 10080),
		Title:      "Provider permissions: recheck interval (minutes)", Description: "How often a stored credential is rechecked so a rotated or narrowed token shows up. 0 disables the scheduled recheck.",
	})
	Register(Definition{
		Key: KeyProvisionCapRefreshMinGap, Category: cat, Kind: KindInt, Default: "60", Env: "APP_PROVISION_CAPABILITIES_REFRESH_MIN_INTERVAL_SECONDS", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 3600),
		Title:      "Provider permissions: forced recheck cooldown (seconds)", Description: "Shortest gap between two forced rechecks of one provider. A faster request gets a 429 with Retry-After.",
	})
	Register(Definition{
		Key: KeyProvisionCapProbeParallel, Category: cat, Kind: KindInt, Default: "4", Env: "APP_PROVISION_CAPABILITIES_PROBE_CONCURRENCY", Apply: ApplyLive,
		CheckValue: intRangeCheck(1, 16),
		Title:      "Provider permissions: parallel probes", Description: "Most read-only probe calls in flight at once, so a check stays far under the provider request limit.",
	})
}
