package runtimesettings

// Keys for the outbound dead man's switch ping.
const (
	KeyAlertingSelfHeartbeatURL      = "alerting.self_heartbeat_url"
	KeyAlertingSelfHeartbeatInterval = "alerting.self_heartbeat_interval"
)

func init() {
	// Sensitive because ping URLs of external monitors usually embed a token in the path.
	Register(Definition{
		Key: KeyAlertingSelfHeartbeatURL, Category: CategoryObservability, Kind: KindString, Default: "",
		Env: "APP_ALERTING_SELF_HEARTBEAT_URL", Apply: ApplyLive, Sensitive: true,
		Title:       "Dead man's switch ping URL",
		Description: "HTTPS URL of an external monitor. The control plane sends it a GET on every interval, so the monitor pages you when the pings stop. Empty turns it off.",
	})
	Register(Definition{
		Key: KeyAlertingSelfHeartbeatInterval, Category: CategoryObservability, Kind: KindInt, Default: "60",
		Env: "APP_ALERTING_SELF_HEARTBEAT_INTERVAL", Apply: ApplyLive, CheckValue: intRangeCheck(10, 86400),
		Title:       "Dead man's switch ping interval (seconds)",
		Description: "Seconds between pings to the dead man's switch URL. Takes effect on the next ping.",
	})
}
