package main

import (
	"context"

	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
)

// setupDNS01Check lets the settings API refuse enabling DNS-01 for every host while no provider is connected.
func (s *bootState) setupDNS01Check() {
	db, sec := s.db, s.secretsManager
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
