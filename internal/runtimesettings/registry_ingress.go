package runtimesettings

import (
	"context"
	"strconv"
	"time"
)

// Registered keys for the ingress body cap and the in-memory access log window.
const (
	KeyIngressMaxBodyBytes    = "ingress.max_body_bytes"
	KeyAccessLogRetention     = "telemetry.access_log_retention"
	ingressBodyCeiling        = 10 << 30
	ingressBodyDefault        = 1 << 30
	minAccessLogRetention     = time.Hour
	maxAccessLogRetention     = 365 * 24 * time.Hour
	defaultAccessLogRetention = maxAccessLogRetention
)

func init() {
	registerLimit(Definition{
		Key: KeyIngressMaxBodyBytes, Category: CategoryNetworking, Default: strconv.Itoa(ingressBodyDefault), Env: "APP_INGRESS_MAX_BODY_BYTES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Ingress request body limit",
		Description: "Largest request body in bytes the ingress accepts for each route before answering HTTP 413, from 1 MiB to 10 GiB. A saved change re-renders the ingress config at once, with no restart; a per-app override still applies. An environment value below the minimum is raised to it and 0 no longer disables the cap.",
	}, bodyLimitFloor, ingressBodyCeiling)
	Register(Definition{
		Key: KeyAccessLogRetention, Category: CategoryObservability, Kind: KindDuration, Default: defaultAccessLogRetention.String(),
		Env: "APP_ACCESS_LOG_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(minAccessLogRetention, maxAccessLogRetention),
		Title:       "Access log retention",
		Description: "How long the recent-requests view keeps a request, from 1h to 8760h; the ring still holds only the latest entries per route. The next read uses a changed value. An environment value of 0 now means the maximum.",
	})
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
