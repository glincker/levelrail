package runtimesettings

import (
	"errors"
	"net/url"
	"strings"
)

// Reachability dial-back keys. The URL empty means nothing is ever contacted.
const (
	KeyDoctorReachabilityDialbackURL          = "doctor.reachability_dialback_url"
	KeyDoctorReachabilityDialbackAllowPrivate = "doctor.reachability_dialback_allow_private"
)

func checkDialbackURL(canonical string) error {
	if strings.TrimSpace(canonical) == "" {
		return nil
	}
	u, err := url.Parse(canonical)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return errors.New("must be an https URL")
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyDoctorReachabilityDialbackURL, Category: CategoryNetworking, Kind: KindString, Default: "",
		Apply: ApplyLive, CheckValue: checkDialbackURL,
		Title:       "Reachability dial-back URL",
		Description: "HTTPS URL of a service that dials this server back from the internet. When set, the reachability check POSTs the list of listening ports as JSON to it, and the service learns this server's public IP from the connection. Empty (the default) means nothing is ever contacted. The service answers with reachable and unreachable port lists.",
	})
	Register(Definition{
		Key: KeyDoctorReachabilityDialbackAllowPrivate, Category: CategoryNetworking, Kind: KindBool, Default: "false",
		Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "Allow private dial-back address",
		Description: "Lets the reachability dial-back URL resolve to a private or loopback address. Off by default so the check cannot be pointed at internal services. Only turn it on for a dial-back service you run on your own network.",
	})
}
