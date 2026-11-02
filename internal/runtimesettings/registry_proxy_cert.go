package runtimesettings

import (
	"context"
	"time"
)

// KeyProxyCertWait is how long a managed proxy route may lack a certificate
// before the diagnosis stops saying "still requesting".
const KeyProxyCertWait = "ingress.proxy_cert_wait"

const (
	minProxyCertWait     = time.Minute
	maxProxyCertWait     = 24 * time.Hour
	defaultProxyCertWait = 5 * time.Minute
)

func init() {
	Register(Definition{
		Key: KeyProxyCertWait, Category: CategoryNetworking, Kind: KindDuration, Default: defaultProxyCertWait.String(),
		Env: "APP_PROXY_CERT_WAIT", Apply: ApplyLive, CheckValue: durationRangeCheck(minProxyCertWait, maxProxyCertWait),
		Title:       "Proxy certificate wait",
		Description: "How long a domain behind an existing proxy may be without a certificate before the status says the proxy is stuck instead of still requesting one, from 1m to 24h. The dashboard also rechecks on its own for this long; a change applies on the next check.",
	})
}

// ProxyCertWait returns the wait forced into its range.
func (s *Service) ProxyCertWait(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyProxyCertWait)
	switch {
	case d < minProxyCertWait:
		return defaultProxyCertWait
	case d > maxProxyCertWait:
		return maxProxyCertWait
	}
	return d
}
