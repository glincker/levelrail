package runtimesettings

import "time"

// Settings for the retry that replaces a stored local-issuer fallback certificate.
const (
	KeyCertFallbackRetryInterval = "ingress.cert_fallback_retry_interval"
	KeyCertFallbackRetryMax      = "ingress.cert_fallback_retry_max"
)

func init() {
	Register(Definition{
		Key: KeyCertFallbackRetryInterval, Category: CategoryNetworking, Kind: KindDuration, Default: "10m", Env: "APP_INGRESS_CERT_FALLBACK_RETRY_INTERVAL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Minute, 6*time.Hour),
		Title:       "Fallback certificate retry interval",
		Description: "How often a domain that holds Caddy's untrusted local fallback certificate, and whose DNS now points at this server, has that certificate dropped so a public certificate is requested again. From 1m to 6h.",
	})
	Register(Definition{
		Key: KeyCertFallbackRetryMax, Category: CategoryNetworking, Kind: KindInt, Default: "12", Env: "APP_INGRESS_CERT_FALLBACK_RETRY_MAX", Apply: ApplyLive,
		CheckValue:  intRangeCheck(1, 1000),
		Title:       "Fallback certificate retry limit",
		Description: "How many times one domain is retried automatically before it waits for a manual renewal or a restart, from 1 to 1000.",
	})
}
