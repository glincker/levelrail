package abuse

import (
	"strconv"
	"strings"
	"time"
)

// Environment variables that tune the abuse defense. Every one is optional.
const (
	EnvMaxRPS            = "APP_ABUSE_MAX_RPS"
	EnvMaxBurst          = "APP_ABUSE_MAX_BURST"
	EnvMaxDenyEntries    = "APP_ABUSE_MAX_DENY_ENTRIES"
	EnvMaxAgentEntries   = "APP_ABUSE_MAX_AGENT_ENTRIES"
	EnvDefaultRPS        = "APP_ABUSE_DEFAULT_RPS"
	EnvDefaultBurst      = "APP_ABUSE_DEFAULT_BURST"
	EnvMaxTrackedClients = "APP_ABUSE_MAX_TRACKED_CLIENTS"
	EnvShards            = "APP_ABUSE_SHARDS"
	EnvIdleEvict         = "APP_ABUSE_IDLE_EVICT"
	EnvMaxRetryAfter     = "APP_ABUSE_MAX_RETRY_AFTER"
	EnvDryRunDefault     = "APP_ABUSE_DRYRUN_DEFAULT_WINDOW"
	EnvDryRunMax         = "APP_ABUSE_DRYRUN_MAX_WINDOW"
	EnvDryRunTop         = "APP_ABUSE_DRYRUN_TOP_CLIENTS"
	EnvSuggestShare      = "APP_ABUSE_SUGGEST_CLIENT_SHARE"
	EnvSuggestBurstSecs  = "APP_ABUSE_SUGGEST_BURST_SECONDS"
)

// Limits caps every policy field and sizes the limiter. Built from the
// environment with LimitsFromEnv.
type Limits struct {
	MaxRPS            float64
	MaxBurst          int
	MaxDenyEntries    int
	MaxAgentEntries   int
	DefaultRPS        float64
	DefaultBurst      int
	MaxTrackedClients int
	Shards            int
	IdleEvict         time.Duration
	MaxRetryAfter     time.Duration
	DryRunDefault     time.Duration
	DryRunMax         time.Duration
	DryRunTop         int
	// SuggestShare is the fraction of the app's peak request rate one client
	// may use before the suggested limit bites; SuggestBurstSeconds sizes the
	// suggested burst as that many seconds of the suggested rate.
	SuggestShare        float64
	SuggestBurstSeconds float64
}

// DefaultLimits are the values used when no environment override is set.
func DefaultLimits() Limits {
	return Limits{
		MaxRPS: 10000, MaxBurst: 100000, MaxDenyEntries: 256, MaxAgentEntries: 64,
		DefaultRPS: 20, DefaultBurst: 40, MaxTrackedClients: 100000, Shards: 64,
		IdleEvict: 5 * time.Minute, MaxRetryAfter: time.Minute,
		DryRunDefault: 15 * time.Minute, DryRunMax: time.Hour, DryRunTop: 10,
		SuggestShare: 0.25, SuggestBurstSeconds: 2,
	}
}

// LimitsFromEnv reads the overrides through getenv; bad or non-positive
// values keep the default.
func LimitsFromEnv(getenv func(string) string) Limits {
	l := DefaultLimits()
	l.MaxRPS = envFloat(getenv, EnvMaxRPS, l.MaxRPS)
	l.MaxBurst = envInt(getenv, EnvMaxBurst, l.MaxBurst)
	l.MaxDenyEntries = envInt(getenv, EnvMaxDenyEntries, l.MaxDenyEntries)
	l.MaxAgentEntries = envInt(getenv, EnvMaxAgentEntries, l.MaxAgentEntries)
	l.DefaultRPS = envFloat(getenv, EnvDefaultRPS, l.DefaultRPS)
	l.DefaultBurst = envInt(getenv, EnvDefaultBurst, l.DefaultBurst)
	l.MaxTrackedClients = envInt(getenv, EnvMaxTrackedClients, l.MaxTrackedClients)
	l.Shards = envInt(getenv, EnvShards, l.Shards)
	l.IdleEvict = envDuration(getenv, EnvIdleEvict, l.IdleEvict)
	l.MaxRetryAfter = envDuration(getenv, EnvMaxRetryAfter, l.MaxRetryAfter)
	l.DryRunDefault = envDuration(getenv, EnvDryRunDefault, l.DryRunDefault)
	l.DryRunMax = envDuration(getenv, EnvDryRunMax, l.DryRunMax)
	l.DryRunTop = envInt(getenv, EnvDryRunTop, l.DryRunTop)
	l.SuggestShare = envFloat(getenv, EnvSuggestShare, l.SuggestShare)
	l.SuggestBurstSeconds = envFloat(getenv, EnvSuggestBurstSecs, l.SuggestBurstSeconds)
	return l
}

func envInt(getenv func(string) string, key string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(getenv(key)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envFloat(getenv func(string) string, key string, def float64) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(getenv(key)), 64)
	if err != nil || f <= 0 {
		return def
	}
	return f
}

func envDuration(getenv func(string) string, key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(getenv(key)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}
