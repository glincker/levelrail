package runtimesettings

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Registered keys for the ingress body cap and the in-memory access log window.
const (
	KeyIngressMaxBodyBytes  = "ingress.max_body_bytes"
	KeyAccessLogRetention   = "telemetry.access_log_retention"
	KeyIngressFallbackCerts = "ingress.fallback_certs"
	// KeyVisitorSaltRotation sets how often the salt behind visitor hashes changes.
	KeyVisitorSaltRotation = "telemetry.visitor_salt_rotation"
	// KeyVisitorHistoryRetention sets how long hourly visitor rollups are kept; 0 turns persistence off.
	KeyVisitorHistoryRetention = "telemetry.visitor_history_retention"
	// FallbackCertsEager and FallbackCertsLazy are the accepted ingress.fallback_certs values.
	FallbackCertsEager         = "eager"
	FallbackCertsLazy          = "lazy"
	ingressBodyCeiling         = 10 << 30
	ingressBodyDefault         = 1 << 30
	minAccessLogRetention      = time.Hour
	maxAccessLogRetention      = 365 * 24 * time.Hour
	defaultAccessLogRetention  = maxAccessLogRetention
	minVisitorSaltRotation     = time.Hour
	maxVisitorSaltRotation     = 30 * 24 * time.Hour
	defaultVisitorSaltRotation = 24 * time.Hour
	maxVisitorHistory          = 90 * 24 * time.Hour
	defaultVisitorHistory      = 7 * 24 * time.Hour
)

func init() {
	registerLimit(Definition{
		Key: KeyIngressMaxBodyBytes, Category: CategoryNetworking, Default: strconv.Itoa(ingressBodyDefault), Env: "APP_INGRESS_MAX_BODY_BYTES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Ingress request body limit",
		Description: "Largest request body in bytes the ingress accepts for each route before answering HTTP 413, from 1 MiB to 10 GiB. A saved change re-renders the ingress config at once, with no restart; a per-app override still applies. An environment value below the minimum is raised to it and 0 no longer disables the cap.",
	}, bodyLimitFloor, ingressBodyCeiling)
	Register(Definition{
		Key: KeyIngressFallbackCerts, Category: CategoryNetworking, Kind: KindString, Default: FallbackCertsEager,
		Env: "APP_INGRESS_FALLBACK_CERTS", Apply: ApplyLive, AllowedStrings: []string{FallbackCertsEager, FallbackCertsLazy},
		Title:       "Fallback domain certificates",
		Description: "When an app has no domain and gets an automatic sslip.io style hostname, eager orders its certificate as soon as the route exists (today's behavior) and lazy orders it on the first TLS request for that hostname. Lazy keeps a burst of new apps from spending certificate rate limits on hosts nobody opens. A saved change re-renders the ingress config at once, with no restart; domains you configured yourself are always eager.",
	})
	Register(Definition{
		Key: KeyAccessLogRetention, Category: CategoryObservability, Kind: KindDuration, Default: defaultAccessLogRetention.String(),
		Env: "APP_ACCESS_LOG_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(minAccessLogRetention, maxAccessLogRetention),
		Title:       "Access log retention",
		Description: "How long the recent-requests view keeps a request, from 1h to 8760h; the ring still holds only the latest entries per route. The next read uses a changed value. An environment value of 0 now means the maximum.",
	})
	Register(Definition{
		Key: KeyVisitorSaltRotation, Category: CategoryObservability, Kind: KindDuration, Default: defaultVisitorSaltRotation.String(),
		Env: "APP_VISITOR_SALT_ROTATION", Apply: ApplyLive, CheckValue: durationRangeCheck(minVisitorSaltRotation, maxVisitorSaltRotation),
		Title:       "Visitor hash salt rotation",
		Description: "How often the salt used to hash client addresses for the visitor counts changes, from 1h to 720h. Addresses are never stored; the same person is counted once per salt period, so a shorter period is more private and counts a returning visitor more than once. The next read uses a changed value.",
	})
	Register(Definition{
		Key: KeyVisitorHistoryRetention, Category: CategoryObservability, Kind: KindDuration, Default: defaultVisitorHistory.String(),
		Env: "APP_VISITOR_HISTORY_RETENTION", Apply: ApplyLive, CheckValue: visitorHistoryCheck,
		Title:       "Visitor history retention",
		Description: "How long hourly visitor rollups (requests, status classes, unique visitors per salt period, a few top pages and referrers) are kept per app and domain so the 7d and 30d visitor windows survive a restart, from 1h to 2160h; 0 keeps nothing beyond the in-memory log. No client address is ever stored. A saved change applies at the next flush and 0 removes the stored rollups.",
	})
}

// visitorHistoryCheck allows 0 (off) or a duration inside the supported range.
func visitorHistoryCheck(canonical string) error {
	d, err := time.ParseDuration(canonical)
	if err != nil || d < 0 || (d > 0 && d < minAccessLogRetention) || d > maxVisitorHistory {
		return fmt.Errorf("value must be 0 (off) or a duration between %s and %s", minAccessLogRetention, maxVisitorHistory)
	}
	return nil
}

// VisitorHistoryRetention returns the rollup window forced into its range; 0 means persistence is off.
func (s *Service) VisitorHistoryRetention(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyVisitorHistoryRetention)
	switch {
	case d <= 0:
		return 0
	case d < minAccessLogRetention:
		return minAccessLogRetention
	case d > maxVisitorHistory:
		return maxVisitorHistory
	}
	return d
}

// VisitorSaltRotation returns the salt period forced into its range.
func (s *Service) VisitorSaltRotation(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyVisitorSaltRotation)
	switch {
	case d < minVisitorSaltRotation:
		return defaultVisitorSaltRotation
	case d > maxVisitorSaltRotation:
		return maxVisitorSaltRotation
	}
	return d
}

// AccessLogRetention returns the access log window forced into its range, so a stray env seed can never hide every record.
func (s *Service) AccessLogRetention(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyAccessLogRetention)
	switch {
	case d <= 0:
		return defaultAccessLogRetention
	case d < minAccessLogRetention:
		return minAccessLogRetention
	case d > maxAccessLogRetention:
		return maxAccessLogRetention
	}
	return d
}
