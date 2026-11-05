package runtimesettings

// KeyPipelinesOIDCListenPort is the port of the job OIDC endpoint on the bridge gateway.
const KeyPipelinesOIDCListenPort = "pipelines.oidc_listen_port"

func init() {
	Register(Definition{
		Key: KeyPipelinesOIDCListenPort, Category: CategoryNetworking, Kind: KindInt, Default: "9095",
		Env: "APP_OIDC_LISTEN_PORT", Apply: ApplyRestart, CheckValue: intRangeCheck(1, 65535),
		Title:       "Pipelines: OIDC listener port",
		Description: "Port of the job OIDC endpoint on the Docker bridge gateway, from 1 to 65535. Change it when a second control plane on the same host holds the default. Read when the listener starts.",
	})
}
