package probe

import (
	"strconv"
	"time"
)

// Limits bounds probe behavior. Zero fields take the package defaults.
type Limits struct {
	MaxRedirects    int
	ExecOutputBytes int
	DefaultTimeout  time.Duration
	DefaultInterval time.Duration
}

// Defaults applied by Limits' zero fields. MaxRedirects matches net/http's
// own client default, which is what probes followed before it was configurable.
const (
	DefaultMaxRedirects    = 10
	DefaultExecOutputBytes = 512
	DefaultTimeout         = 2 * time.Second
	DefaultInterval        = 2 * time.Second
)

// Environment variables LimitsFromEnv reads.
const (
	EnvMaxRedirects    = "APP_PROBE_MAX_REDIRECTS"
	EnvExecOutputBytes = "APP_PROBE_EXEC_OUTPUT_BYTES"
	EnvDefaultTimeout  = "APP_PROBE_DEFAULT_TIMEOUT"
	EnvDefaultInterval = "APP_PROBE_DEFAULT_INTERVAL"
)

// LimitsFromEnv reads Limits from lookup (os.LookupEnv in production).
// Unset or malformed values fall back to the defaults.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	return Limits{
		MaxRedirects:    envInt(lookup, EnvMaxRedirects),
		ExecOutputBytes: envInt(lookup, EnvExecOutputBytes),
		DefaultTimeout:  envDuration(lookup, EnvDefaultTimeout),
		DefaultInterval: envDuration(lookup, EnvDefaultInterval),
	}.withDefaults()
}

func (l Limits) withDefaults() Limits {
	if l.MaxRedirects <= 0 {
		l.MaxRedirects = DefaultMaxRedirects
	}
	if l.ExecOutputBytes <= 0 {
		l.ExecOutputBytes = DefaultExecOutputBytes
	}
	if l.DefaultTimeout <= 0 {
		l.DefaultTimeout = DefaultTimeout
	}
	if l.DefaultInterval <= 0 {
		l.DefaultInterval = DefaultInterval
	}
	return l
}

func envInt(lookup func(string) (string, bool), key string) int {
	raw, ok := lookup(key)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func envDuration(lookup func(string) (string, bool), key string) time.Duration {
	raw, ok := lookup(key)
	if !ok {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return d
}
