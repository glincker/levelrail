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
// probing, otherwise the first public IPv4 literal any probe URL returns,
// falling back to IPv6 only when no IPv4 answer arrives. Never blocks longer than APP_PUBLIC_IP_PROBE_TIMEOUT.
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
	// IPv4 first: an sslip.io name built from an IPv6 literal has no A record,
	// and ACME validators then have no IPv4 address to fall back to.
	for _, network := range []string{"tcp4", "tcp"} {
		if ip := probePublicIP(ctx, urls, timeout, network); ip != "" {
			return ip, PublicHostSourceDetected
		}
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
func probePublicIP(ctx context.Context, urls []string, timeout time.Duration, network string) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	found := make(chan string, len(urls))
	client := newProbeClient(network)
	for _, u := range urls {
		go func(u string) { found <- fetchPublicIP(ctx, client, u) }(u)
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

// newProbeClient dials only over network ("tcp4" forces IPv4).
func newProbeClient(network string) *http.Client {
	dialer := &net.Dialer{}
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}}
}

func fetchPublicIP(ctx context.Context, client *http.Client, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
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
