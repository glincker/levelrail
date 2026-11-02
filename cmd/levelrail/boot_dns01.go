package main

import (
	"context"
	"time"

	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// setupDNS01Check lets the settings API refuse enabling DNS-01 for every host while no provider is connected.
func (s *bootState) setupDNS01Check() {
	db, sec := s.db, s.secretsManager
	s.ingressDriver.SetCARetryBackoff(func(c context.Context) (time.Duration, time.Duration) {
		rs := s.apiRouter.RuntimeSettings()
		return rs.Duration(c, runtimesettings.KeyIngressCARetryBase), rs.Duration(c, runtimesettings.KeyIngressCARetryMax)
	})
	s.apiRouter.SetDNS01ProviderCheck(func(ctx context.Context) bool {
		if sec == nil {
			return false
		}
		return ingressreconcile.DNSProviderConnected(ctx, db,
			ingressreconcile.WithCloudflareDNSTokens(sec),
			ingressreconcile.WithRoute53DNSCredentials(sec),
			ingressreconcile.WithGoogleCloudDNSKey(sec))
	})
}
