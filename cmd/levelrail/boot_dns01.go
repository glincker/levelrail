package main

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/certfallback"
	"github.com/glincker/levelrail/internal/cloudflareips"
	"github.com/glincker/levelrail/internal/ingress"
	ingressreconcile "github.com/glincker/levelrail/internal/reconcile/ingress"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

// setupDNS01Check lets the settings API refuse enabling DNS-01 for every host while no provider is connected.
func (s *bootState) setupDNS01Check() {
	db, sec := s.db, s.secretsManager
	s.ingressDriver.SetCARetryBackoff(func(c context.Context) (time.Duration, time.Duration) {
		rs := s.apiRouter.RuntimeSettings()
		return rs.Duration(c, runtimesettings.KeyIngressCARetryBase), rs.Duration(c, runtimesettings.KeyIngressCARetryMax)
	})
	s.startCloudflareIPsRefresh()
	s.startCertFallbackRetry()
	s.apiRouter.SetDNS01ProviderCheck(func(ctx context.Context) bool {
		if sec == nil {
			return false
		}
		return ingressreconcile.DNSProviderConnected(ctx, db,
			ingressreconcile.WithCloudflareDNSTokens(sec),
			ingressreconcile.WithRoute53DNSCredentials(sec),
			ingressreconcile.WithGoogleCloudDNSKey(sec),
			ingressreconcile.WithDNSTokens(sec))
	})
}

// startCloudflareIPsRefresh keeps the Cloudflare range list current; every knob is re-read from stored settings each round.
func (s *bootState) startCloudflareIPsRefresh() {
	rs := s.apiRouter.RuntimeSettings()
	limits := func(c context.Context) cloudflareips.Limits {
		return cloudflareips.Limits{
			Timeout:   rs.Duration(c, runtimesettings.KeyCloudflareIPsTimeout),
			MaxBytes:  int64(rs.Int(c, runtimesettings.KeyCloudflareIPsMaxBytes)),
			MinRanges: rs.Int(c, runtimesettings.KeyCloudflareIPsMinRanges),
		}
	}
	next := func(c context.Context) time.Duration { return rs.Duration(c, runtimesettings.KeyCloudflareIPsRefresh) }
	go cloudflareips.Default().Run(s.ctx, limits, next, s.engine.Nudge)
}

// startCertFallbackRetry re-requests a public certificate for domains stuck on the local fallback once their DNS points here.
func (s *bootState) startCertFallbackRetry() {
	rs, router := s.apiRouter.RuntimeSettings(), s.apiRouter
	r := &certfallback.Retrier{
		Store:      s.db,
		Candidates: router.CertFallbackCandidates,
		Resolves: func(c context.Context, domain string) bool {
			return certfallback.ResolvesHere(c, domain, ingress.DetectPublicIPs(c))
		},
		Interval: func(c context.Context) time.Duration {
			return rs.Duration(c, runtimesettings.KeyCertFallbackRetryInterval)
		},
		MaxRetries: func(c context.Context) int { return rs.Int(c, runtimesettings.KeyCertFallbackRetryMax) },
		Audit: func(c context.Context, domain, detail string) {
			router.AuditCertFallbackDrop(c, "system", "cert-fallback", "fallback certificate retry", domain, detail)
		},
		Nudge: s.engine.Nudge,
		Evict: s.ingressDriver.ResetCertCache,
	}
	go r.Run(s.ctx)
}
