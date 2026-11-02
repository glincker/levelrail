package runtimesettings

import (
	"context"
	"time"
)

// Cloudflare client IP preset keys and the refresh bounds around its range list.
const (
	KeyIngressTrustedProxyPreset = "ingress.trusted_proxy_preset"
	KeyCloudflareIPsRefresh      = "ingress.cloudflare_ips_refresh"
	KeyCloudflareIPsMinRanges    = "ingress.cloudflare_ips_min_ranges"
	KeyCloudflareIPsTimeout      = "ingress.cloudflare_ips_timeout"
	KeyCloudflareIPsMaxBytes     = "ingress.cloudflare_ips_max_bytes"

	// TrustedProxyPresetNone and TrustedProxyPresetCloudflare are the preset values.
	TrustedProxyPresetNone       = "none"
	TrustedProxyPresetCloudflare = "cloudflare"
)

func init() {
	Register(Definition{
		Key: KeyIngressTrustedProxyPreset, Category: CategoryNetworking, Kind: KindString, Default: TrustedProxyPresetNone,
		Env: "APP_INGRESS_TRUSTED_PROXY_PRESET", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		AllowedStrings: []string{TrustedProxyPresetNone, TrustedProxyPresetCloudflare},
		StrengthOrder:  []string{TrustedProxyPresetCloudflare, TrustedProxyPresetNone},
		Title:          "Trusted proxy preset",
		Description:    "cloudflare makes the ingress take the visitor address from CF-Connecting-IP, but only when the connection comes from Cloudflare's published ranges (kept current in the background); a spoofed header from any other peer is ignored. Any Cloudflare customer can reach your server through a proxied record of their own, so this does not prove a request came through your zone. APP_INGRESS_TRUSTED_PROXIES entries stay trusted as well.",
	})
	Register(Definition{
		Key: KeyCloudflareIPsRefresh, Category: CategoryNetworking, Kind: KindDuration, Default: "24h",
		Env: "APP_CLOUDFLARE_IPS_REFRESH", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Hour, 30*24*time.Hour),
		Title:       "Cloudflare ranges refresh interval",
		Description: "How often the Cloudflare address list is fetched from Cloudflare's official endpoint, from 1h to 720h. The list shipped in the binary is used until the first fetch succeeds, and the last good list is kept when a fetch fails.",
	})
	Register(Definition{
		Key: KeyCloudflareIPsMinRanges, Category: CategoryNetworking, Kind: KindInt, Default: "10",
		Env: "APP_CLOUDFLARE_IPS_MIN_RANGES", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000),
		Title:       "Cloudflare ranges sanity floor",
		Description: "A fetched Cloudflare list with fewer ranges than this is treated as truncated and ignored, so a bad answer cannot shrink the trusted set.",
	})
	Register(Definition{
		Key: KeyCloudflareIPsTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "15s",
		Env: "APP_CLOUDFLARE_IPS_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, 2*time.Minute),
		Title:       "Cloudflare ranges fetch timeout",
		Description: "How long one fetch of the Cloudflare address list may take, from 1s to 2m.",
	})
	Register(Definition{
		Key: KeyCloudflareIPsMaxBytes, Category: CategoryNetworking, Kind: KindInt, Default: "1048576",
		Env: "APP_CLOUDFLARE_IPS_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(4096, 16<<20),
		Title:       "Cloudflare ranges answer size limit",
		Description: "Largest answer in bytes accepted from the Cloudflare address endpoint, from 4096 to 16 MiB.",
	})
}

// TrustedProxyPreset returns the stored preset, falling back to none for any unknown value.
func (s *Service) TrustedProxyPreset(ctx context.Context) string {
	if s.String(ctx, KeyIngressTrustedProxyPreset) == TrustedProxyPresetCloudflare {
		return TrustedProxyPresetCloudflare
	}
	return TrustedProxyPresetNone
}
