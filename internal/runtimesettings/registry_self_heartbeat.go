package runtimesettings

import (
	"errors"
	"net/url"
	"strings"

	"github.com/glincker/levelrail/kit/netguard"
)

// Bounds of the dead man's switch interval, in seconds.
const (
	SelfHeartbeatMinInterval = 10
	SelfHeartbeatMaxInterval = 86400
)

// checkSelfHeartbeatURL vets the URL at save time without any DNS lookup.
func checkSelfHeartbeatURL(canonical string) error {
	v := strings.TrimSpace(canonical)
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Hostname() == "" {
		return errors.New("value must be an absolute URL with a host")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !netguard.AllowPrivate()) {
		return errors.New("value must be an https URL")
	}
	return nil
}

// Keys for the outbound dead man's switch ping.
const (
	KeyAlertingSelfHeartbeatURL      = "alerting.self_heartbeat_url"
	KeyAlertingSelfHeartbeatInterval = "alerting.self_heartbeat_interval"
)

func init() {
	// Sensitive because ping URLs of external monitors usually embed a token in the path.
	Register(Definition{
		Key: KeyAlertingSelfHeartbeatURL, Category: CategoryObservability, Kind: KindString, Default: "",
		Env: "APP_ALERTING_SELF_HEARTBEAT_URL", Apply: ApplyLive, Sensitive: true, CheckValue: checkSelfHeartbeatURL,
		Title:       "Dead man's switch ping URL",
		Description: "HTTPS URL of an external monitor. The control plane sends it a GET on every interval, so the monitor pages you when the pings stop. Empty turns it off.",
	})
	Register(Definition{
		Key: KeyAlertingSelfHeartbeatInterval, Category: CategoryObservability, Kind: KindInt, Default: "60",
		Env: "APP_ALERTING_SELF_HEARTBEAT_INTERVAL", Apply: ApplyLive, CheckValue: intRangeCheck(SelfHeartbeatMinInterval, SelfHeartbeatMaxInterval),
		Title:       "Dead man's switch ping interval (seconds)",
		Description: "Seconds between pings to the dead man's switch URL. Takes effect on the next ping.",
	})
}
