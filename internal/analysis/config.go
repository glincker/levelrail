// Package analysis builds deterministic incident and cost reports from
// existing telemetry (ADR 051, phase one). It never calls a model and is
// never part of reconciliation.
package analysis

import (
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

// Config holds every threshold, read from APP_ANALYSIS_* so operators tune them.
type Config struct {
	Enabled         bool
	Window          time.Duration
	Baseline        time.Duration
	AnomalyZ        float64
	TrendPct        float64
	UtilizationPct  float64
	RestartMin      int
	LogErrorMin     int
	LogGroups       int
	LogSampleBytes  int
	MaxEvents       int
	DeployProximity time.Duration
	MinInterval     time.Duration
	MaxConcurrent   int
	Timeout         time.Duration
	Retention       time.Duration
	// Sizing feeds the right-sizing engine; a zero field keeps the engine default.
	Sizing rightsizing.Tuning
}

// ConfigFromEnv reads the knobs. Analysis stays off unless APP_ANALYSIS_ENABLED is "on".
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	get := func(k string) string { v, _ := lookup(k); return v }
	dur := func(k string, def time.Duration) time.Duration {
		if d, err := time.ParseDuration(get(k)); err == nil && d > 0 {
			return d
		}
		return def
	}
	num := func(k string, def float64) float64 {
		if f, err := strconv.ParseFloat(get(k), 64); err == nil && f > 0 {
			return f
		}
		return def
	}
	return Config{
		Enabled:         get("APP_ANALYSIS_ENABLED") == "on",
		Window:          dur("APP_ANALYSIS_WINDOW", 6*time.Hour),
		Baseline:        dur("APP_ANALYSIS_BASELINE_WINDOW", 24*time.Hour),
		AnomalyZ:        num("APP_ANALYSIS_ANOMALY_Z", 3.5),
		TrendPct:        num("APP_ANALYSIS_TREND_PCT", 10),
		UtilizationPct:  num("APP_ANALYSIS_UTILIZATION_PCT", 85),
		RestartMin:      int(num("APP_ANALYSIS_RESTART_THRESHOLD", 3)),
		LogErrorMin:     int(num("APP_ANALYSIS_LOG_ERROR_MIN", 5)),
		LogGroups:       int(num("APP_ANALYSIS_LOG_GROUPS", 10)),
		LogSampleBytes:  int(num("APP_ANALYSIS_LOG_SAMPLE_BYTES", 200)),
		MaxEvents:       int(num("APP_ANALYSIS_MAX_EVENTS", 50)),
		DeployProximity: dur("APP_ANALYSIS_DEPLOY_PROXIMITY", time.Hour),
		MinInterval:     dur("APP_ANALYSIS_MIN_INTERVAL", 10*time.Minute),
		MaxConcurrent:   int(num("APP_ANALYSIS_MAX_CONCURRENT", 2)),
		Timeout:         dur("APP_ANALYSIS_TIMEOUT", 30*time.Second),
		Retention:       dur("APP_ANALYSIS_RETENTION", 30*24*time.Hour),
		Sizing: rightsizing.Tuning{
			MemoryMargin: num("APP_ANALYSIS_HEADROOM_MEMORY", 0),
			CPUMargin:    num("APP_ANALYSIS_HEADROOM_CPU", 0),
			RaiseAt:      num("APP_ANALYSIS_SIZING_RAISE_AT", 0),
			LowerAt:      num("APP_ANALYSIS_SIZING_LOWER_AT", 0),
			MinCoverage:  dur("APP_ANALYSIS_SIZING_MIN_COVERAGE", 0),
		},
	}
}
