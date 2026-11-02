package ingress

import "slices"

// HeaderCFConnectingIP carries the visitor address Cloudflare saw.
const HeaderCFConnectingIP = "CF-Connecting-IP"

// WithCloudflarePreset adds Cloudflare's ranges to the trusted proxies and
// reads the client address from CF-Connecting-IP first. Caddy only honours the
// header from a trusted peer, so a spoofed one from anywhere else is ignored.
// Operator-listed proxies and their headers stay in force.
func (h Hardening) WithCloudflarePreset(cidrs []string) Hardening {
	if len(cidrs) == 0 {
		return h
	}
	trusted := slices.Clone(h.TrustedProxies)
	for _, c := range cidrs {
		if !slices.Contains(trusted, c) {
			trusted = append(trusted, c)
		}
	}
	h.TrustedProxies = trusted
	headers := []string{HeaderCFConnectingIP}
	if len(h.ClientIPHeaders) > 0 {
		headers = append(headers, h.ClientIPHeaders...)
	} else if len(h.TrustedProxies) > len(cidrs) {
		headers = append(headers, "X-Forwarded-For")
	}
	h.ClientIPHeaders = slices.Compact(headers)
	return h
}
