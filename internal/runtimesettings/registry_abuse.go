package runtimesettings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/abuse"
)

// Registered keys for the per-app abuse defense limits.
const (
	KeyAbuseMaxRPS            = "abuse.max_rps"
	KeyAbuseMaxBurst          = "abuse.max_burst"
	KeyAbuseMaxDenyEntries    = "abuse.max_deny_entries"
	KeyAbuseMaxAgentEntries   = "abuse.max_agent_entries"
	KeyAbuseDefaultRPS        = "abuse.default_rps"
	KeyAbuseDefaultBurst      = "abuse.default_burst"
	KeyAbuseMaxTrackedClients = "abuse.max_tracked_clients"
	KeyAbuseShards            = "abuse.shards"
	KeyAbuseIdleEvict         = "abuse.idle_evict"
	KeyAbuseMaxRetryAfter     = "abuse.max_retry_after"
	KeyAbuseDryRunDefault     = "abuse.dryrun_default_window"
	KeyAbuseDryRunMax         = "abuse.dryrun_max_window"
	KeyAbuseDryRunTop         = "abuse.dryrun_top_clients"
	KeyAbuseSuggestShare      = "abuse.suggest_client_share"
	KeyAbuseSuggestBurstSecs  = "abuse.suggest_burst_seconds"
)

type abuseFloatRange struct{ lo, hi float64 }

type abuseDurationRange struct{ lo, hi time.Duration }

var (
	abuseFloats    = map[string]abuseFloatRange{}
	abuseDurations = map[string]abuseDurationRange{}
)

func abuseFloatCheck(r abuseFloatRange) func(string) error {
	return func(canonical string) error {
		f, err := strconv.ParseFloat(canonical, 64)
		if err != nil || f < r.lo || f > r.hi {
			return fmt.Errorf("value must be a number between %g and %g", r.lo, r.hi)
		}
		return nil
	}
}

func registerAbuseFloat(d Definition, lo, hi float64) {
	d.Kind = KindString
	r := abuseFloatRange{lo: lo, hi: hi}
	d.CheckValue = abuseFloatCheck(r)
	abuseFloats[d.Key] = r
	Register(d)
}

func registerAbuseDuration(d Definition, lo, hi time.Duration) {
	d.Kind = KindDuration
	d.CheckValue = durationRangeCheck(lo, hi)
	abuseDurations[d.Key] = abuseDurationRange{lo: lo, hi: hi}
	Register(d)
}

func abuseDefault(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func init() {
	def := abuse.DefaultLimits()
	itoa := strconv.Itoa
	registerAbuseFloat(Definition{
		Key: KeyAbuseMaxRPS, Category: CategorySecurity, Default: abuseDefault(def.MaxRPS), Env: abuse.EnvMaxRPS, Apply: ApplyLive,
		Title:       "Abuse defense: highest requests per second",
		Description: "Ceiling for the per-client rate any app's abuse policy may set, from 1 to 1000000 requests per second. A policy above it is rejected when saved. Applies to the next policy save and the next proxy refresh.",
	}, 1, 1_000_000)
	registerLimit(Definition{
		Key: KeyAbuseMaxBurst, Category: CategorySecurity, Default: itoa(def.MaxBurst), Env: abuse.EnvMaxBurst, Apply: ApplyLive,
		Title:       "Abuse defense: largest burst",
		Description: "Ceiling for the burst size any app's abuse policy may set, from 1 to 10000000 requests. Applies to the next policy save and the next proxy refresh.",
	}, 1, 10_000_000)
	registerLimit(Definition{
		Key: KeyAbuseMaxDenyEntries, Category: CategorySecurity, Default: itoa(def.MaxDenyEntries), Env: abuse.EnvMaxDenyEntries, Apply: ApplyLive,
		Title:       "Abuse defense: most deny entries",
		Description: "Most address or network entries one app's deny list may hold, from 1 to 10000. Applies to the next policy save.",
	}, 1, 10_000)
	registerLimit(Definition{
		Key: KeyAbuseMaxAgentEntries, Category: CategorySecurity, Default: itoa(def.MaxAgentEntries), Env: abuse.EnvMaxAgentEntries, Apply: ApplyLive,
		Title:       "Abuse defense: most user agent rules",
		Description: "Most user agent rules one app's abuse policy may hold, from 1 to 1000. Applies to the next policy save.",
	}, 1, 1_000)
	registerAbuseFloat(Definition{
		Key: KeyAbuseDefaultRPS, Category: CategorySecurity, Default: abuseDefault(def.DefaultRPS), Env: abuse.EnvDefaultRPS, Apply: ApplyLive,
		Title:       "Abuse defense: default requests per second",
		Description: "Per-client rate used when a policy turns rate limiting on without naming one, from 0.1 to 1000000. It cannot exceed the highest rate. Applies to the next policy save and the next proxy refresh.",
	}, 0.1, 1_000_000)
	registerLimit(Definition{
		Key: KeyAbuseDefaultBurst, Category: CategorySecurity, Default: itoa(def.DefaultBurst), Env: abuse.EnvDefaultBurst, Apply: ApplyLive,
		Title:       "Abuse defense: default burst",
		Description: "Burst size used when a policy turns rate limiting on without naming one, from 1 to 10000000. It cannot exceed the largest burst. Applies to the next policy save and the next proxy refresh.",
	}, 1, 10_000_000)
	registerLimit(Definition{
		Key: KeyAbuseMaxTrackedClients, Category: CategorySecurity, Default: itoa(def.MaxTrackedClients), Env: abuse.EnvMaxTrackedClients, Apply: ApplyRestart,
		Title:       "Abuse defense: clients tracked per app",
		Description: "How many distinct client addresses each app's limiter remembers at once, from 1000 to 10000000. The table is sized when the proxy starts, so a change needs a restart.",
	}, 1_000, 10_000_000)
	registerLimit(Definition{
		Key: KeyAbuseShards, Category: CategorySecurity, Default: itoa(def.Shards), Env: abuse.EnvShards, Apply: ApplyRestart,
		Title:       "Abuse defense: limiter shards",
		Description: "How many lock shards each app's limiter splits its client table into, from 1 to 1024. More shards cut contention on busy apps. Set when the proxy starts, so a change needs a restart.",
	}, 1, 1_024)
	registerAbuseDuration(Definition{
		Key: KeyAbuseIdleEvict, Category: CategorySecurity, Default: def.IdleEvict.String(), Env: abuse.EnvIdleEvict, Apply: ApplyRestart,
		Title:       "Abuse defense: idle client eviction",
		Description: "How long a client can stay quiet before the limiter forgets it, from 10s to 24h. Set when the proxy starts, so a change needs a restart.",
	}, 10*time.Second, 24*time.Hour)
	registerAbuseDuration(Definition{
		Key: KeyAbuseMaxRetryAfter, Category: CategorySecurity, Default: def.MaxRetryAfter.String(), Env: abuse.EnvMaxRetryAfter, Apply: ApplyLive,
		Title:       "Abuse defense: longest retry hint",
		Description: "Upper bound on the Retry-After value sent with a rate limit rejection, from 1s to 1h. Applies at the next proxy refresh.",
	}, time.Second, time.Hour)
	registerAbuseDuration(Definition{
		Key: KeyAbuseDryRunDefault, Category: CategorySecurity, Default: def.DryRunDefault.String(), Env: abuse.EnvDryRunDefault, Apply: ApplyLive,
		Title:       "Abuse defense: default dry run window",
		Description: "How long a dry run watches traffic when none is named, from 1m to 24h. It cannot exceed the longest dry run window. Applies to the next dry run.",
	}, time.Minute, 24*time.Hour)
	registerAbuseDuration(Definition{
		Key: KeyAbuseDryRunMax, Category: CategorySecurity, Default: def.DryRunMax.String(), Env: abuse.EnvDryRunMax, Apply: ApplyLive,
		Title:       "Abuse defense: longest dry run window",
		Description: "Longest window a dry run may watch traffic, from 1m to 168h. Applies to the next dry run.",
	}, time.Minute, 168*time.Hour)
	registerLimit(Definition{
		Key: KeyAbuseDryRunTop, Category: CategorySecurity, Default: itoa(def.DryRunTop), Env: abuse.EnvDryRunTop, Apply: ApplyLive,
		Title:       "Abuse defense: dry run top clients",
		Description: "How many of the busiest clients a dry run report lists, from 1 to 100. Applies to the next dry run.",
	}, 1, 100)
	registerAbuseFloat(Definition{
		Key: KeyAbuseSuggestShare, Category: CategorySecurity, Default: abuseDefault(def.SuggestShare), Env: abuse.EnvSuggestShare, Apply: ApplyLive,
		Title:       "Abuse defense: suggested client share",
		Description: "Fraction of an app's peak request rate that one client may use before the suggested limit applies, from 0.01 to 1. Applies to the next suggestion.",
	}, 0.01, 1)
	registerAbuseFloat(Definition{
		Key: KeyAbuseSuggestBurstSecs, Category: CategorySecurity, Default: abuseDefault(def.SuggestBurstSeconds), Env: abuse.EnvSuggestBurstSecs, Apply: ApplyLive,
		Title:       "Abuse defense: suggested burst seconds",
		Description: "The suggested burst is this many seconds of the suggested rate, from 0.1 to 60. Applies to the next suggestion.",
	}, 0.1, 60)
}

// abuseFloatSetting returns a float setting forced into its range; a bad env seed falls back to the default.
func (s *Service) abuseFloatSetting(ctx context.Context, key string, fallback float64) float64 {
	r := abuseFloats[key]
	f, err := strconv.ParseFloat(strings.TrimSpace(s.String(ctx, key)), 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return min(max(f, r.lo), r.hi)
}

func (s *Service) abuseDurationSetting(ctx context.Context, key string, fallback time.Duration) time.Duration {
	r := abuseDurations[key]
	d := s.Duration(ctx, key)
	if d <= 0 {
		return fallback
	}
	return min(max(d, r.lo), r.hi)
}

// AbuseLimits builds the abuse limits from the saved settings, env seeds and defaults, so a caller re-reading it
// per pass sees a changed value without a restart. Restart-only keys return their boot value.
func (s *Service) AbuseLimits(ctx context.Context) abuse.Limits {
	def := abuse.DefaultLimits()
	return abuse.Limits{
		MaxRPS:              s.abuseFloatSetting(ctx, KeyAbuseMaxRPS, def.MaxRPS),
		MaxBurst:            s.Limit(ctx, KeyAbuseMaxBurst),
		MaxDenyEntries:      s.Limit(ctx, KeyAbuseMaxDenyEntries),
		MaxAgentEntries:     s.Limit(ctx, KeyAbuseMaxAgentEntries),
		DefaultRPS:          s.abuseFloatSetting(ctx, KeyAbuseDefaultRPS, def.DefaultRPS),
		DefaultBurst:        s.Limit(ctx, KeyAbuseDefaultBurst),
		MaxTrackedClients:   s.Limit(ctx, KeyAbuseMaxTrackedClients),
		Shards:              s.Limit(ctx, KeyAbuseShards),
		IdleEvict:           s.abuseDurationSetting(ctx, KeyAbuseIdleEvict, def.IdleEvict),
		MaxRetryAfter:       s.abuseDurationSetting(ctx, KeyAbuseMaxRetryAfter, def.MaxRetryAfter),
		DryRunDefault:       s.abuseDurationSetting(ctx, KeyAbuseDryRunDefault, def.DryRunDefault),
		DryRunMax:           s.abuseDurationSetting(ctx, KeyAbuseDryRunMax, def.DryRunMax),
		DryRunTop:           s.Limit(ctx, KeyAbuseDryRunTop),
		SuggestShare:        s.abuseFloatSetting(ctx, KeyAbuseSuggestShare, def.SuggestShare),
		SuggestBurstSeconds: s.abuseFloatSetting(ctx, KeyAbuseSuggestBurstSecs, def.SuggestBurstSeconds),
	}
}
