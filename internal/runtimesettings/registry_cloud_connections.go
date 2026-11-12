package runtimesettings

// KeyCloudConnectionsEnabled gates the cloud connections API and CLI (preview).
const KeyCloudConnectionsEnabled = "cloud.connections_enabled"

func init() {
	Register(Definition{
		Key: KeyCloudConnectionsEnabled, Category: CategoryExperimental, Kind: KindBool, Default: "false",
		Env: "APP_CLOUD_CONNECTIONS_ENABLED", Apply: ApplyLive,
		Title:       "Cloud connections (preview)",
		Description: "Preview. Allow storing named cloud account connections and running read-only health checks. Credentials stay in your own vault or secret store and are only referenced. Off by default; while off the cloud connection API refuses every request.",
	})
}
