package runtimesettings

import (
	"context"
	"time"
)

// Keys for the limits on Pages proxy routes.
const (
	KeyPagesProxyMaxBodyMB       = "pages.proxy_max_body_mb"
	KeyPagesProxyResponseTimeout = "pages.proxy_response_timeout"
)

func init() {
	Register(Definition{
		Key: KeyPagesProxyMaxBodyMB, Category: CategoryNetworking, Kind: KindInt, Default: "10",
		Env: "APP_PAGES_PROXY_MAX_BODY_MB", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1024),
		Title:       "Pages: proxy request body cap",
		Description: "Largest request body in MiB a static site proxy route forwards to its target app. Applies at the next ingress refresh.",
	})
	Register(Definition{
		Key: KeyPagesProxyResponseTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "30s",
		Env: "APP_PAGES_PROXY_RESPONSE_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, 10*time.Minute),
		Title:       "Pages: proxy response timeout",
		Description: "How long a static site proxy route waits for the target app to start answering before it returns a 502. WebSocket upgrades are unaffected after the handshake. Applies at the next ingress refresh.",
	})
}

// PagesProxyLimits returns the saved body cap in bytes and response timeout.
func (s *Service) PagesProxyLimits(ctx context.Context) (int64, time.Duration) {
	mb := s.Int(ctx, KeyPagesProxyMaxBodyMB)
	d := s.Duration(ctx, KeyPagesProxyResponseTimeout)
	return int64(mb) << 20, d
}
