package runtimesettings

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// KeyDomainsAuthGateTimeout is the default auth gate subrequest timeout.
const KeyDomainsAuthGateTimeout = "domains.auth_gate_timeout"

const (
	authGateTimeoutFloor = time.Second
	authGateTimeoutCeil  = time.Minute
)

// authGateEnvSeconds lets the long-standing whole-seconds env form keep working.
func authGateEnvSeconds(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if _, err := strconv.Atoi(raw); err == nil {
		return raw + "s", nil
	}
	return raw, nil
}

func init() {
	Register(Definition{
		Key: KeyDomainsAuthGateTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "5s", Env: "APP_AUTH_GATE_TIMEOUT", Apply: ApplyLive,
		EnvConvert: authGateEnvSeconds, CheckValue: durationRangeCheck(authGateTimeoutFloor, authGateTimeoutCeil),
		Title:       "Domains: auth gate timeout",
		Description: "How long the ingress waits for an auth gate's check before it denies the request, from 1s to 1m. A domain's own timeout wins. Applies at the next ingress refresh.",
	})
}

// AuthGateTimeout returns the saved default forced into its range, so a bad seed never disables the wait.
func (s *Service) AuthGateTimeout(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyDomainsAuthGateTimeout)
	if d <= 0 {
		return 5 * time.Second
	}
	return min(max(d, authGateTimeoutFloor), authGateTimeoutCeil)
}
