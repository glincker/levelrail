package ingress

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Public host sources reported alongside the resolved address.
const (
	PublicHostSourceEnv      = "env"
	PublicHostSourceDetected = "detected"
	PublicHostSourceDisabled = "disabled"
	PublicHostSourceNone     = "none"
)

// Env vars governing public address discovery. None bakes in the product name.
const (
	envPublicHost         = "APP_PUBLIC_HOST"
	envPublicIPDetect     = "APP_PUBLIC_IP_DETECT"
	envPublicIPProbeURLs  = "APP_PUBLIC_IP_PROBE_URLS"
	envPublicIPProbeLimit = "APP_PUBLIC_IP_PROBE_TIMEOUT"
)

var defaultPublicIPProbeURLs = []string{
	"https://api.ipify.org",
	"https://icanhazip.com",
	"https://ifconfig.me/ip",
}

const defaultPublicIPProbeTimeout = 4 * time.Second

// ResolvePublicHost returns the address apps are reachable at and where it
// came from: APP_PUBLIC_HOST wins, APP_PUBLIC_IP_DETECT=off disables
// probing, otherwise the first public IPv4/IPv6 literal any probe URL
// returns. Never blocks longer than APP_PUBLIC_IP_PROBE_TIMEOUT.
func ResolvePublicHost(ctx context.Context) (host, source string) {
	if v := strings.TrimSpace(os.Getenv(envPublicHost)); v != "" {
		return v, PublicHostSourceEnv
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envPublicIPDetect))) {
	case "0", "false", "off", "no", "disabled":
		return "", PublicHostSourceDisabled
	}
	urls := defaultPublicIPProbeURLs
	if raw := strings.TrimSpace(os.Getenv(envPublicIPProbeURLs)); raw != "" {
		urls = splitNonEmpty(raw)
	}
	timeout := defaultPublicIPProbeTimeout
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(envPublicIPProbeLimit))); err == nil && d > 0 {
		timeout = d
	}
	if ip := probePublicIP(ctx, urls, timeout); ip != "" {
		return ip, PublicHostSourceDetected
	}
	return "", PublicHostSourceNone
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// probePublicIP races every URL; the first valid public IP wins.
func probePublicIP(ctx context.Context, urls []string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	found := make(chan string, len(urls))
	for _, u := range urls {
		go func(u string) { found <- fetchPublicIP(ctx, u) }(u)
	}
	for range urls {
		select {
		case ip := <-found:
			if ip != "" {
				return ip
			}
		case <-ctx.Done():
			return ""
		}
	}
	return ""
}

func fetchPublicIP(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(string(body)))
	if ip == nil || !isPubliclyRoutable(ip) {
		return ""
	}
	return ip.String()
}

// SSLIPHost is the zero-DNS hostname for the server itself:
// "<dashed-ip>.sslip.io". ok is false when publicHost is not a public IP.
func SSLIPHost(publicHost string) (host string, ok bool) {
	ip := net.ParseIP(strings.TrimSpace(publicHost))
	if ip == nil || !isPubliclyRoutable(ip) {
		return "", false
	}
	return dashEncodeIP(ip) + "." + fallbackDomainSuffix, true
}
