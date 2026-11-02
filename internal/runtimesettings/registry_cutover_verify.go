package runtimesettings

import (
	"context"
	"time"
)

// Registered keys for the domain cutover verifier thresholds.
const (
	KeyCutoverVerifyCertWarnDays = "cutover.verify_cert_warn_days"
	KeyCutoverVerifyCertFailDays = "cutover.verify_cert_fail_days"
	KeyCutoverVerifySlowTTFB     = "cutover.verify_slow_ttfb"
	KeyCutoverVerifyHSTSMinAge   = "cutover.verify_hsts_min_age"
	KeyCutoverVerifyTimeout      = "cutover.verify_timeout"
	KeyCutoverVerifyMinInterval  = "cutover.verify_min_interval"
)

const (
	minVerifySlowTTFB = 100 * time.Millisecond
	maxVerifySlowTTFB = 30 * time.Second
	minVerifyTimeout  = time.Second
	maxVerifyTimeout  = time.Minute
	minVerifyInterval = time.Second
	maxVerifyInterval = time.Hour
)

func init() {
	Register(Definition{
		Key: KeyCutoverVerifyCertWarnDays, Category: CategoryNetworking, Kind: KindInt, Default: "21",
		Env: "APP_CUTOVER_VERIFY_CERT_WARN_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 365),
		Title:       "Cutover verify: certificate warning days",
		Description: "Days left on a domain's certificate at or below which the cutover verifier shows a warning, from 1 to 365. It should be at least the failure threshold; if it is lower, the failure threshold is used. The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyCutoverVerifyCertFailDays, Category: CategoryNetworking, Kind: KindInt, Default: "7",
		Env: "APP_CUTOVER_VERIFY_CERT_FAIL_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 365),
		Title:       "Cutover verify: certificate failure days",
		Description: "Days left on a domain's certificate at or below which the cutover verifier fails the certificate line, from 1 to 365. The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyCutoverVerifySlowTTFB, Category: CategoryNetworking, Kind: KindDuration, Default: "1500ms",
		Env: "APP_CUTOVER_VERIFY_SLOW_TTFB", Apply: ApplyLive, CheckValue: durationRangeCheck(minVerifySlowTTFB, maxVerifySlowTTFB),
		Title:       "Cutover verify: slow first byte",
		Description: "Time to first byte above which the cutover verifier warns about the response, from 100ms to 30s. The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyCutoverVerifyHSTSMinAge, Category: CategoryNetworking, Kind: KindInt, Default: "15552000",
		Env: "APP_CUTOVER_VERIFY_HSTS_MIN_AGE_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(3600, 63072000),
		Title:       "Cutover verify: minimum HSTS age",
		Description: "Smallest HSTS max-age in seconds that the cutover verifier counts as enabled, from 3600 to 63072000 (two years). The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyCutoverVerifyTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "5s",
		Env: "APP_CUTOVER_VERIFY_REQUEST_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(minVerifyTimeout, maxVerifyTimeout),
		Title:       "Cutover verify: request timeout",
		Description: "How long each request of the cutover verifier may take, from 1s to 1m. The whole check is allowed four times this long. It is separate from the wait after a scheduled cutover. The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyCutoverVerifyMinInterval, Category: CategoryNetworking, Kind: KindDuration, Default: "10s",
		Env: "APP_CUTOVER_VERIFY_MIN_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(minVerifyInterval, maxVerifyInterval),
		Title:       "Cutover verify: minimum interval",
		Description: "Shortest wait before the same person can run the cutover verifier again on the same domain, from 1s to 1h. Other people can check that domain after a quarter of this wait. The next check uses a changed value.",
	})
}

// CutoverVerify holds the verifier thresholds, forced into their safe ranges
// so a stray env seed can never turn a line off.
type CutoverVerify struct {
	CertWarnDays int
	CertFailDays int
	SlowTTFB     time.Duration
	HSTSMinAge   int
	Timeout      time.Duration
	MinInterval  time.Duration
}

// CutoverVerify reads the thresholds at call time.
func (s *Service) CutoverVerify(ctx context.Context) CutoverVerify {
	v := CutoverVerify{
		CertWarnDays: clampInt(s.Int(ctx, KeyCutoverVerifyCertWarnDays), 1, 365),
		CertFailDays: clampInt(s.Int(ctx, KeyCutoverVerifyCertFailDays), 1, 365),
		SlowTTFB:     clampDuration(s.Duration(ctx, KeyCutoverVerifySlowTTFB), minVerifySlowTTFB, maxVerifySlowTTFB),
		HSTSMinAge:   clampInt(s.Int(ctx, KeyCutoverVerifyHSTSMinAge), 3600, 63072000),
		Timeout:      clampDuration(s.Duration(ctx, KeyCutoverVerifyTimeout), minVerifyTimeout, maxVerifyTimeout),
		MinInterval:  clampDuration(s.Duration(ctx, KeyCutoverVerifyMinInterval), minVerifyInterval, maxVerifyInterval),
	}
	v.CertWarnDays = max(v.CertWarnDays, v.CertFailDays)
	return v
}

func clampInt(n, lo, hi int) int { return min(max(n, lo), hi) }

func clampDuration(d, lo, hi time.Duration) time.Duration { return min(max(d, lo), hi) }
