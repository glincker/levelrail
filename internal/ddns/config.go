// Package ddns keeps a dynamic DNS record pointed at this host's public IPv4.
package ddns

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env names. The updater is off unless APP_DDNS_ENABLED is on and a target is set.
const (
	EnvEnabled        = "APP_DDNS_ENABLED"
	EnvInterval       = "APP_DDNS_INTERVAL"
	EnvProbeURLs      = "APP_DDNS_PROBE_URLS"
	EnvProbeQuorum    = "APP_DDNS_PROBE_QUORUM"
	EnvTimeout        = "APP_DDNS_TIMEOUT"
	EnvMinUpdateGap   = "APP_DDNS_MIN_UPDATE_GAP"
	EnvReassert       = "APP_DDNS_REASSERT_INTERVAL"
	EnvFailAlertAfter = "APP_DDNS_FAIL_ALERT_AFTER"
	EnvAllowPrivate   = "APP_DDNS_ALLOW_PRIVATE"
	EnvAllowInsecure  = "APP_DDNS_ALLOW_INSECURE_HTTP"
	EnvUpdateURL      = "APP_DDNS_UPDATE_URL"
	EnvDuckDomains    = "APP_DDNS_DUCKDNS_DOMAINS"
	EnvDuckToken      = "APP_DDNS_DUCKDNS_TOKEN" //nolint:gosec // env var name, not a credential

	// Shared with boot-time detection so there is one probe list.
	envSharedProbeURLs = "APP_PUBLIC_IP_PROBE_URLS"
	envPublicIPDetect  = "APP_PUBLIC_IP_DETECT"
)

// ErrNoTarget means the updater is enabled but no provider is configured.
var ErrNoTarget = errors.New("ddns: enabled but no update target configured")

// Config holds the loop settings; zero durations are replaced by defaults.
type Config struct {
	Interval       time.Duration
	ProbeURLs      []string
	Quorum         int
	Timeout        time.Duration
	MinUpdateGap   time.Duration
	Reassert       time.Duration
	FailAlertAfter int
	AllowPrivate   bool
	AllowInsecure  bool
}

// ConfigFromEnv reads the loop settings; getenv is injectable for tests.
func ConfigFromEnv(getenv func(string) string) Config {
	urls := splitList(getenv(EnvProbeURLs))
	if len(urls) == 0 {
		urls = splitList(getenv(envSharedProbeURLs))
	}
	if len(urls) == 0 {
		urls = []string{"https://api.ipify.org", "https://icanhazip.com", "https://ifconfig.me/ip"}
	}
	return Config{
		Interval:       dur(getenv(EnvInterval), 5*time.Minute),
		ProbeURLs:      urls,
		Quorum:         num(getenv(EnvProbeQuorum), 2),
		Timeout:        dur(getenv(EnvTimeout), 10*time.Second),
		MinUpdateGap:   dur(getenv(EnvMinUpdateGap), 5*time.Minute),
		Reassert:       dur(getenv(EnvReassert), 24*time.Hour),
		FailAlertAfter: num(getenv(EnvFailAlertAfter), 3),
		AllowPrivate:   on(getenv(EnvAllowPrivate)),
		AllowInsecure:  on(getenv(EnvAllowInsecure)),
	}
}

// Enabled reports whether the operator turned the updater on and left detection enabled.
func Enabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(envPublicIPDetect))) {
	case "0", "false", "off", "no", "disabled":
		return false
	}
	return on(getenv(EnvEnabled))
}

// ProviderFromEnv builds the configured provider: the generic update URL wins, then the DuckDNS preset.
func ProviderFromEnv(getenv func(string) string, cfg Config) (Provider, error) {
	if raw := strings.TrimSpace(getenv(EnvUpdateURL)); raw != "" {
		return NewHTTPProvider(raw, cfg, nil)
	}
	domains, token := strings.TrimSpace(getenv(EnvDuckDomains)), strings.TrimSpace(getenv(EnvDuckToken))
	if domains != "" && token != "" {
		return NewDuckDNS(domains, token, cfg)
	}
	return nil, ErrNoTarget
}

// OSEnv reads the process environment.
func OSEnv(k string) string { return os.Getenv(k) }

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dur(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(s)); err == nil && d > 0 {
		return d
	}
	return def
}

func num(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
		return n
	}
	return def
}

func on(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "on", "yes", "enabled":
		return true
	}
	return false
}

// escape is shared by the DuckDNS preset when it builds its template.
func escape(s string) string { return url.QueryEscape(s) }
