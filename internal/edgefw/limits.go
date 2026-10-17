// Package edgefw is the ingress firewall: ordered rules, automatic bans,
// bot modes, a proof-of-work challenge and bounded in-memory visibility.
package edgefw

import (
	"os"
	"strconv"
	"time"
)

// Environment variables that tune the firewall. All are optional.
const (
	EnvMaxRules       = "APP_FIREWALL_MAX_RULES"
	EnvMaxRateEntries = "APP_FIREWALL_MAX_RATE_ENTRIES"
	EnvBanStrikes     = "APP_FIREWALL_BAN_STRIKES"
	EnvBanWindow      = "APP_FIREWALL_BAN_WINDOW"
	EnvBanDuration    = "APP_FIREWALL_BAN_DURATION"
	EnvMaxBans        = "APP_FIREWALL_MAX_BANS"
	EnvChallengeBits  = "APP_FIREWALL_CHALLENGE_BITS"
	EnvChallengeTTL   = "APP_FIREWALL_CHALLENGE_TTL"
	EnvPassTTL        = "APP_FIREWALL_PASS_TTL" //nolint:gosec // env var name
	EnvTopCapacity    = "APP_FIREWALL_TOP_CAPACITY"
	EnvMaxStatScopes  = "APP_FIREWALL_MAX_STAT_HOSTS"
	EnvTopSample      = "APP_FIREWALL_TOP_SAMPLE"
	EnvSuggestRate    = "APP_FIREWALL_SUGGEST_ATTACK_PER_MINUTE"
	EnvSuggestMinutes = "APP_FIREWALL_SUGGEST_ATTACK_MINUTES"
	EnvMaxBotAgents   = "APP_FIREWALL_MAX_AI_BOT_AGENTS"
)

// Limits bounds every table the firewall keeps and sets its defaults.
type Limits struct {
	MaxRules       int
	MaxRateEntries int
	BanStrikes     int
	BanWindow      time.Duration
	BanDuration    time.Duration
	MaxBans        int
	ChallengeBits  int
	ChallengeTTL   time.Duration
	PassTTL        time.Duration
	TopCapacity    int
	MaxStatScopes  int
	TopSample      int
	SuggestRate    int
	SuggestMinutes int
	MaxBotAgents   int
}

// DefaultLimits are the values used when no variable is set.
func DefaultLimits() Limits {
	return Limits{
		MaxRules: 500, MaxRateEntries: 10000,
		BanStrikes: 20, BanWindow: 5 * time.Minute, BanDuration: time.Hour, MaxBans: 10000,
		ChallengeBits: 16, ChallengeTTL: 2 * time.Minute, PassTTL: 30 * time.Minute,
		TopCapacity: 20, MaxStatScopes: 64, TopSample: 1,
		SuggestRate: 120, SuggestMinutes: 3, MaxBotAgents: 256,
	}
}

// LimitsFromEnv overlays set variables on DefaultLimits; unparseable or
// non-positive values keep the default.
func LimitsFromEnv(getenv func(string) string) Limits {
	if getenv == nil {
		getenv = os.Getenv
	}
	l := DefaultLimits()
	ints := []struct {
		key string
		dst *int
	}{
		{EnvMaxRules, &l.MaxRules}, {EnvMaxRateEntries, &l.MaxRateEntries}, {EnvBanStrikes, &l.BanStrikes},
		{EnvMaxBans, &l.MaxBans}, {EnvChallengeBits, &l.ChallengeBits}, {EnvTopCapacity, &l.TopCapacity},
		{EnvMaxStatScopes, &l.MaxStatScopes}, {EnvTopSample, &l.TopSample}, {EnvSuggestRate, &l.SuggestRate},
		{EnvSuggestMinutes, &l.SuggestMinutes}, {EnvMaxBotAgents, &l.MaxBotAgents},
	}
	for _, v := range ints {
		if n, err := strconv.Atoi(getenv(v.key)); err == nil && n > 0 {
			*v.dst = n
		}
	}
	durs := []struct {
		key string
		dst *time.Duration
	}{
		{EnvBanWindow, &l.BanWindow}, {EnvBanDuration, &l.BanDuration},
		{EnvChallengeTTL, &l.ChallengeTTL}, {EnvPassTTL, &l.PassTTL},
	}
	for _, v := range durs {
		if d, err := time.ParseDuration(getenv(v.key)); err == nil && d > 0 {
			*v.dst = d
		}
	}
	if l.ChallengeBits > 28 {
		l.ChallengeBits = 28
	}
	return l
}
