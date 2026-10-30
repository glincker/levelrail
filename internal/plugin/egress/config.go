package egress

import (
	"strconv"
	"strings"
	"time"
)

// Environment variable names read by ConfigFromEnv.
const (
	EnvConnectTimeout  = "LEVELRAIL_PLUGIN_EGRESS_CONNECT_TIMEOUT"
	EnvIdleTimeout     = "LEVELRAIL_PLUGIN_EGRESS_IDLE_TIMEOUT"
	EnvMaxTunnels      = "LEVELRAIL_PLUGIN_EGRESS_MAX_TUNNELS"
	EnvBytesPerMinute  = "LEVELRAIL_PLUGIN_EGRESS_BYTES_PER_MINUTE"
	EnvAllowPrivate    = "LEVELRAIL_PLUGIN_EGRESS_ALLOW_PRIVATE"
	defaultConnect     = 10 * time.Second
	defaultIdle        = 60 * time.Second
	defaultMaxTunnels  = 16
	defaultBytesPerMin = int64(256 << 20)
)

// Config holds the proxy limits. Zero values mean "use the default".
type Config struct {
	ConnectTimeout time.Duration
	IdleTimeout    time.Duration
	// MaxTunnels caps simultaneous connections per plugin.
	MaxTunnels int
	// BytesPerMinute caps traffic per plugin in both directions combined.
	BytesPerMinute int64
	// AllowPrivate lets targets that resolve to internal addresses through.
	// The caller decides it, usually from EnvAllowPrivate.
	AllowPrivate bool
}

// ConfigFromEnv builds a Config from LEVELRAIL_PLUGIN_EGRESS_* variables.
// Unparseable values are ignored so a typo keeps the safe default.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	var c Config
	if d, err := time.ParseDuration(get(EnvConnectTimeout)); err == nil && d > 0 {
		c.ConnectTimeout = d
	}
	if d, err := time.ParseDuration(get(EnvIdleTimeout)); err == nil && d > 0 {
		c.IdleTimeout = d
	}
	if n, err := strconv.Atoi(get(EnvMaxTunnels)); err == nil && n > 0 {
		c.MaxTunnels = n
	}
	if n, err := strconv.ParseInt(get(EnvBytesPerMinute), 10, 64); err == nil && n > 0 {
		c.BytesPerMinute = n
	}
	if b, err := strconv.ParseBool(get(EnvAllowPrivate)); err == nil {
		c.AllowPrivate = b
	}
	return c
}

func (c Config) withDefaults() Config {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = defaultConnect
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdle
	}
	if c.MaxTunnels <= 0 {
		c.MaxTunnels = defaultMaxTunnels
	}
	if c.BytesPerMinute <= 0 {
		c.BytesPerMinute = defaultBytesPerMin
	}
	return c
}

// StaticAllowlist is a simple Allowlist. Entries are "host" (port 443 only)
// or "host:port" (that port only); hosts match exactly, ignoring case.
type StaticAllowlist map[string][]string

// Allowed implements Allowlist.
func (s StaticAllowlist) Allowed(plugin, host string, port int) bool {
	host = strings.ToLower(host)
	for _, e := range s[plugin] {
		e = strings.ToLower(strings.TrimSpace(e))
		eh, ep := e, 443
		if i := strings.LastIndexByte(e, ':'); i > 0 {
			n, err := strconv.Atoi(e[i+1:])
			if err != nil {
				continue
			}
			eh, ep = e[:i], n
		}
		if eh == host && ep == port {
			return true
		}
	}
	return false
}
