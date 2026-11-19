package runtimesettings

// Build transfer settings. The env vars only seed the defaults; a saved value
// applies to the next dispatched build without a restart.
const (
	KeyBuildTransferMode    = "build.transfer_mode"
	KeyBuildTransferPushTTL = "build.transfer_push_ttl"
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
}
