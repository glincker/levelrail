package runtimesettings

// KeyMonitorsEnabled turns the uptime monitor engine on or off without a restart.
const KeyMonitorsEnabled = "monitors.enabled"

func init() {
	Register(Definition{
		Key: KeyMonitorsEnabled, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_MONITORS_ENABLED", Apply: ApplyLive,
		Title:       "Uptime monitors",
		Description: "Run uptime monitors (http, tcp, tls, dns, ping and cron heartbeat checks). Turning it on starts the engine and turning it off stops it within seconds; saved monitors and history are kept.",
	})
}
