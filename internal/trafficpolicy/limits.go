package trafficpolicy

import (
	"os"
	"strconv"
	"time"
)

// Environment variables that tune the caps below. Every one is optional.
const (
	EnvMaxHeaderRules     = "APP_DOMAIN_POLICY_MAX_HEADER_RULES"
	EnvMaxForwarders      = "APP_DOMAIN_POLICY_MAX_FORWARDERS"
	EnvMaxCacheRules      = "APP_DOMAIN_POLICY_MAX_CACHE_RULES"
	EnvMaxRegexLen        = "APP_DOMAIN_POLICY_MAX_REGEX_LEN"
	EnvMaxHeaderValueLen  = "APP_DOMAIN_POLICY_MAX_HEADER_VALUE_LEN"
	EnvMaxExemptEntries   = "APP_DOMAIN_POLICY_MAX_GEO_EXEMPT"
	EnvForwardTimeout     = "APP_FORWARDER_TIMEOUT"
	EnvForwardMaxTimeout  = "APP_FORWARDER_MAX_TIMEOUT"
	EnvForwardDialTimeout = "APP_FORWARDER_DIAL_TIMEOUT"
	EnvCacheMaxObject     = "APP_INGRESS_CACHE_MAX_OBJECT_BYTES"
	EnvCacheMaxTTL        = "APP_INGRESS_CACHE_MAX_TTL"
	EnvMaxErrorBody       = "APP_DOMAIN_POLICY_MAX_BODY_BYTES"
)

// Limits caps the size of every policy section. Built from the environment
// with LimitsFromEnv; DefaultLimits is the zero-config value.
type Limits struct {
	MaxHeaderRules     int
	MaxForwarders      int
	MaxCacheRules      int
	MaxRegexLen        int
	MaxHeaderValueLen  int
	MaxListEntries     int
	MaxExemptEntries   int
	MaxBodyBytes       int
	ForwardTimeout     time.Duration
	ForwardMaxTimeout  time.Duration
	ForwardDialTimeout time.Duration
	CacheMaxObject     int64
	CacheMaxTTL        time.Duration
}

// DefaultLimits are the caps used when no environment override is set.
func DefaultLimits() Limits {
	return Limits{
		MaxHeaderRules:     50,
		MaxForwarders:      50,
		MaxCacheRules:      20,
		MaxRegexLen:        256,
		MaxHeaderValueLen:  4096,
		MaxListEntries:     64,
		MaxExemptEntries:   256,
		MaxBodyBytes:       64 << 10,
		ForwardTimeout:     30 * time.Second,
		ForwardMaxTimeout:  10 * time.Minute,
		ForwardDialTimeout: 5 * time.Second,
		CacheMaxObject:     2 << 20,
		CacheMaxTTL:        7 * 24 * time.Hour,
	}
}

// LimitsFromEnv overlays any set environment variable on DefaultLimits.
// Unparseable or non-positive values keep the default.
func LimitsFromEnv(getenv func(string) string) Limits {
	if getenv == nil {
		getenv = os.Getenv
	}
	l := DefaultLimits()
	intVar(getenv, EnvMaxHeaderRules, &l.MaxHeaderRules)
	intVar(getenv, EnvMaxForwarders, &l.MaxForwarders)
	intVar(getenv, EnvMaxCacheRules, &l.MaxCacheRules)
	intVar(getenv, EnvMaxRegexLen, &l.MaxRegexLen)
	intVar(getenv, EnvMaxHeaderValueLen, &l.MaxHeaderValueLen)
	intVar(getenv, EnvMaxExemptEntries, &l.MaxExemptEntries)
	intVar(getenv, EnvMaxErrorBody, &l.MaxBodyBytes)
	durVar(getenv, EnvForwardTimeout, &l.ForwardTimeout)
	durVar(getenv, EnvForwardMaxTimeout, &l.ForwardMaxTimeout)
	durVar(getenv, EnvForwardDialTimeout, &l.ForwardDialTimeout)
	durVar(getenv, EnvCacheMaxTTL, &l.CacheMaxTTL)
	if v, err := strconv.ParseInt(getenv(EnvCacheMaxObject), 10, 64); err == nil && v > 0 {
		l.CacheMaxObject = v
	}
	return l
}

func intVar(getenv func(string) string, key string, dst *int) {
	if v, err := strconv.Atoi(getenv(key)); err == nil && v > 0 {
		*dst = v
	}
}

func durVar(getenv func(string) string, key string, dst *time.Duration) {
	if v, err := time.ParseDuration(getenv(key)); err == nil && v > 0 {
		*dst = v
	}
}
