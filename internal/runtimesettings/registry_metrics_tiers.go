package runtimesettings

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Keys for metric rollup tier retention, query ranges and the maintenance job (ADR 080).
// Raw retention keeps its older key, KeyMetricsRetention, so saved values carry over.
const (
	KeyMetricsRetention1m  = "metrics.retention_1m"
	KeyMetricsRetention5m  = "metrics.retention_5m"
	KeyMetricsRetention1h  = "metrics.retention_1h"
	KeyMetricsRetention1d  = "metrics.retention_1d"
	KeyMetricsRawRange     = "metrics.raw_query_max_range"
	KeyMetricsRange1m      = "metrics.1m_query_max_range"
	KeyMetricsRange5m      = "metrics.5m_query_max_range"
	KeyMetricsRange1h      = "metrics.1h_query_max_range"
	KeyMetricsMaxDBBytes   = "metrics.max_db_bytes"
	KeyMetricsRollupLag    = "metrics.rollup_lag"
	KeyMetricsRollupEvery  = "metrics.rollup_every"
	KeyMetricsPruneEvery   = "metrics.prune_every"
	KeyMetricsRollupBudget = "metrics.rollup_budget"
)

const (
	maxMetricsTierRetention  = 87600 * time.Hour
	minMetricsTierRange      = time.Minute
	metricsRawBucket         = time.Minute
	metricsMinBytesCap       = int64(16 << 20)
	metricsRollupMaxInterval = 24 * time.Hour
)

// MetricsTiers is the live view of the tier settings. The 5m and 1d fields,
// the 1h range and the budget are stored but nothing reads them until those tiers ship.
type MetricsTiers struct {
	RawRetention, Retention1m, Retention5m, Retention1h, Retention1d time.Duration
	RawRange, Range1m, Range5m, Range1h                              time.Duration
	MaxDBBytes                                                       int64
	Lag, RollupEvery, PruneEvery, RollupBudget                       time.Duration
}

// MetricsTiers reads every tier setting; each call sees the latest saved value.
func (s *Service) MetricsTiers(ctx context.Context) MetricsTiers {
	d := func(k string) time.Duration { return s.Duration(ctx, k) }
	n, _ := strconv.ParseInt(s.value(ctx, KeyMetricsMaxDBBytes), 10, 64)
	return MetricsTiers{
		RawRetention: d(KeyMetricsRetention), Retention1m: d(KeyMetricsRetention1m), Retention5m: d(KeyMetricsRetention5m),
		Retention1h: d(KeyMetricsRetention1h), Retention1d: d(KeyMetricsRetention1d),
		RawRange: d(KeyMetricsRawRange), Range1m: d(KeyMetricsRange1m), Range5m: d(KeyMetricsRange5m), Range1h: d(KeyMetricsRange1h),
		MaxDBBytes: n, Lag: d(KeyMetricsRollupLag), RollupEvery: d(KeyMetricsRollupEvery),
		PruneEvery: d(KeyMetricsPruneEvery), RollupBudget: d(KeyMetricsRollupBudget),
	}
}

func maxBytesCheck(canonical string) error {
	n, err := strconv.ParseInt(canonical, 10, 64)
	if err != nil || (n != 0 && n < metricsMinBytesCap) {
		return fmt.Errorf("value must be 0 (no cap) or at least %d bytes (16 MiB)", metricsMinBytesCap)
	}
	return nil
}

func init() {
	const inactive = " Not used until the 5 minute and 1 day tiers ship, so a saved value has no effect yet."
	const order = " A save is rejected when it breaks the rule that each tier keeps its query range plus one bucket and each coarser tier outlasts the finer one."
	retention := func(key, def, env, title, desc string) {
		Register(Definition{
			Key: key, Category: CategoryObservability, Kind: KindDuration, Default: def, Env: env, Apply: ApplyLive,
			CheckValue: durationRangeCheck(time.Hour, maxMetricsTierRetention), Title: title, Description: desc,
		})
	}
	rng := func(key, def, env, title, desc string) {
		Register(Definition{
			Key: key, Category: CategoryObservability, Kind: KindDuration, Default: def, Env: env, Apply: ApplyLive,
			CheckValue: durationRangeCheck(minMetricsTierRange, maxMetricsTierRetention), Title: title, Description: desc,
		})
	}
	retention(KeyMetricsRetention1m, "720h", "APP_METRICS_RETENTION_1M", "1 minute metrics retention",
		"How long 1 minute rollup rows are kept, from 1h to 87600h. The default keeps the history an upgrade already has."+order)
	retention(KeyMetricsRetention5m, "72h", "APP_METRICS_RETENTION_5M", "5 minute metrics retention",
		"How long 5 minute rollup rows are kept, from 1h to 87600h."+inactive+order)
	retention(KeyMetricsRetention1h, "8760h", "APP_METRICS_RETENTION_1H", "1 hour metrics retention",
		"How long 1 hour rollup rows are kept, from 1h to 87600h. The default keeps the history an upgrade already has."+order)
	retention(KeyMetricsRetention1d, "9600h", "APP_METRICS_RETENTION_1D", "1 day metrics retention",
		"How long 1 day rollup rows are kept, from 1h to 87600h."+inactive+order)
	rng(KeyMetricsRawRange, "6h", "APP_METRICS_RAW_QUERY_MAX_RANGE", "Raw metrics query range",
		"Widest chart range served from raw samples, from 1m to 87600h; wider ranges read a rollup tier."+order)
	rng(KeyMetricsRange1m, "168h", "APP_METRICS_MINUTE_QUERY_MAX_RANGE", "1 minute metrics query range",
		"Widest chart range served from 1 minute rows, from 1m to 87600h; wider ranges read the 1 hour tier."+order)
	rng(KeyMetricsRange5m, "72h", "APP_METRICS_5M_QUERY_MAX_RANGE", "5 minute metrics query range",
		"Widest chart range served from 5 minute rows, from 1m to 87600h."+inactive+order)
	rng(KeyMetricsRange1h, "840h", "APP_METRICS_1H_QUERY_MAX_RANGE", "1 hour metrics query range",
		"Widest chart range served from 1 hour rows, from 1m to 87600h."+inactive+order)
	Register(Definition{
		Key: KeyMetricsMaxDBBytes, Category: CategoryObservability, Kind: KindInt, Default: "1073741824", Env: "APP_METRICS_MAX_DB_BYTES",
		Apply: ApplyLive, EnvLoose: true, CheckValue: maxBytesCheck,
		Title:       "Metrics database size cap",
		Description: "Bytes the metrics database may grow to before pruning removes the oldest rows early, 0 for no cap, otherwise at least 16 MiB. A saved change applies to the next prune.",
	})
	Register(Definition{
		Key: KeyMetricsRollupLag, Category: CategoryObservability, Kind: KindDuration, Default: "30s", Env: "APP_METRICS_ROLLUP_LAG", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Second, time.Hour),
		Title:       "Metrics rollup lag",
		Description: "How long a bucket stays open after its end so late samples still land in it, from 1s to 1h. A saved change applies to the next rollup.",
	})
	Register(Definition{
		Key: KeyMetricsRollupEvery, Category: CategoryObservability, Kind: KindDuration, Default: "1m", Env: "APP_METRICS_ROLLUP_EVERY", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(10*time.Second, metricsRollupMaxInterval),
		Title:       "Metrics rollup interval",
		Description: "How often the rollup job runs, from 10s to 24h. A saved change applies after the current wait ends.",
	})
	Register(Definition{
		Key: KeyMetricsPruneEvery, Category: CategoryObservability, Kind: KindDuration, Default: "1h", Env: "APP_METRICS_PRUNE_EVERY", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Minute, metricsRollupMaxInterval),
		Title:       "Metrics prune interval",
		Description: "How often the retention sweep runs, from 1m to 24h. A saved change applies after the current wait ends.",
	})
	Register(Definition{
		Key: KeyMetricsRollupBudget, Category: CategoryObservability, Kind: KindDuration, Default: "5s", Env: "APP_METRICS_ROLLUP_BUDGET", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Second, time.Hour),
		Title:       "Metrics rollup time budget",
		Description: "Longest one rollup run works before it stops and resumes next run, from 1s to 1h." + inactive,
	})
}

// tierRule is one sizing or ordering rule over the settings it names. It is
// checked only when every named setting is active or was set on purpose.
type tierRule struct {
	keys []string
	ok   func(v map[string]time.Duration) string
}

// tierActive lists the tier settings something reads today; the rest only take part once set.
var tierActive = map[string]bool{
	KeyMetricsRetention: true, KeyMetricsRetention1m: true, KeyMetricsRetention1h: true,
	KeyMetricsRawRange: true, KeyMetricsRange1m: true,
}

func fitRule(retention, rng string, bucket time.Duration) tierRule {
	return tierRule{keys: []string{retention, rng}, ok: func(v map[string]time.Duration) string {
		if v[retention] < v[rng]+bucket {
			return fmt.Sprintf("%s (%s) must be at least %s (%s) plus one bucket of %s", retention, v[retention], rng, v[rng], bucket)
		}
		return ""
	}}
}

func outlastRule(finer, coarser string) tierRule {
	return tierRule{keys: []string{finer, coarser}, ok: func(v map[string]time.Duration) string {
		if v[coarser] <= v[finer] {
			return fmt.Sprintf("%s (%s) must be longer than %s (%s)", coarser, v[coarser], finer, v[finer])
		}
		return ""
	}}
}

var tierRules = []tierRule{
	fitRule(KeyMetricsRetention, KeyMetricsRawRange, metricsRawBucket),
	fitRule(KeyMetricsRetention1m, KeyMetricsRange1m, time.Minute),
	fitRule(KeyMetricsRetention5m, KeyMetricsRange5m, 5*time.Minute),
	fitRule(KeyMetricsRetention1h, KeyMetricsRange1h, time.Hour),
	outlastRule(KeyMetricsRetention, KeyMetricsRetention1m),
	outlastRule(KeyMetricsRetention, KeyMetricsRetention5m),
	outlastRule(KeyMetricsRetention1m, KeyMetricsRetention1h),
	outlastRule(KeyMetricsRetention5m, KeyMetricsRetention1h),
	outlastRule(KeyMetricsRetention1h, KeyMetricsRetention1d),
}

// checkTierSet validates candidate for key against the effective values of the
// other tier settings, so one save cannot leave the set inconsistent.
func (s *Service) checkTierSet(ctx context.Context, key, candidate string) error {
	cand, err := time.ParseDuration(candidate)
	if err != nil {
		return nil
	}
	vals := map[string]time.Duration{key: cand}
	use := func(k string) bool {
		if k == key {
			return true
		}
		if _, ok := s.definition(k); !ok {
			return false
		}
		if tierActive[k] || s.Source(ctx, k) != SourceDefault {
			vals[k] = durationOf(s.value(ctx, k))
			return true
		}
		return false
	}
	for _, r := range tierRules {
		if !touches(r.keys, key) {
			continue
		}
		all := true
		for _, k := range r.keys {
			all = use(k) && all
		}
		if !all {
			continue
		}
		if msg := r.ok(vals); msg != "" {
			return &ValidationError{Key: key, Message: msg}
		}
	}
	return nil
}

func touches(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}
