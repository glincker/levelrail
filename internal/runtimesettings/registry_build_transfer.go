package runtimesettings

// Build transfer settings. The env vars only seed the defaults; a saved value
// applies to the next dispatched build without a restart.
const (
	KeyBuildTransferMode    = "build.transfer_mode"
	KeyBuildTransferPushTTL = "build.transfer_push_ttl"
	// KeyBuildTransferBearerTTL is the lifetime of each registry bearer; builds refresh it, so revocation applies within it.
	KeyBuildTransferBearerTTL = "build.transfer_bearer_ttl"
	// KeyRegistryTokenRateLimit throttles the registry token endpoint apart from sign-in limits.
	KeyRegistryTokenRateLimit = "registry.token_rate_limit_rpm"
)

func init() {
	Register(Definition{
		Key: KeyBuildTransferMode, Category: CategoryDeploys, Kind: KindString, Default: "tar", Env: "APP_BUILD_TRANSFER_MODE", Apply: ApplyLive,
		AllowedStrings: []string{"tar", "registry"},
		Title:          "Remote build image transfer",
		Description:    "How a build on another node returns its image: tar streams it back over the agent connection, registry has the node push it to the built-in registry and the control plane pulls it by digest. Registry needs the built-in registry, token auth and agents that support it; anything missing falls back to tar. Applies to the next build.",
	})
	Register(Definition{
		Key: KeyBuildTransferPushTTL, Category: CategoryDeploys, Kind: KindDuration, Default: "45m", Env: "APP_BUILD_TRANSFER_PUSH_TTL", Apply: ApplyLive,
		Title:       "Registry push token lifetime",
		Description: "How long a build's registry push token stays valid. Set it to the longest build time plus queue wait and a little slack. The token is also revoked when the build ends.",
	})
	Register(Definition{
		Key: KeyBuildTransferBearerTTL, Category: CategoryDeploys, Kind: KindDuration, Default: "90s", Env: "APP_BUILD_TRANSFER_BEARER_TTL", Apply: ApplyLive,
		Title:       "Registry bearer token lifetime",
		Description: "How long each registry bearer token stays valid. Builds refresh it through the token endpoint, so revoking a build's push token takes effect within this time.",
	})
	registerLimit(Definition{
		Key: KeyRegistryTokenRateLimit, Category: CategorySecurity, Default: "120", Env: "APP_REGISTRY_TOKEN_RATE_LIMIT_RPM", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Registry token endpoint rate limit",
		Description: "Registry token requests per minute allowed per client address and build. Requests over the budget get HTTP 429. Minimum 10; a change applies at once.",
	}, 10, rateLimitCeiling)
}
