package build

import "sort"

// Speed finding codes. The dashboard translates these.
const (
	SpeedCacheDisabled     = "cache_disabled"
	SpeedCacheUnconfigured = "cache_unconfigured"
	SpeedLowCacheHit       = "low_cache_hit"
	SpeedSlowerRecently    = "slower_recently"
)

// Fix kinds: auto is applied by the control plane, guided needs one action elsewhere, manual is a repo change.
const (
	SpeedFixAuto   = "auto"
	SpeedFixGuided = "guided"
	SpeedFixManual = "manual"
)

const (
	minRunsForCacheAdvice = 3
	minRunsForRegression  = 6
	lowCacheHitRatio      = 0.5
	regressionFactor      = 1.3
	warmRatio             = 0.8
	coldRatio             = 0.2
)

// SpeedCacheState is what the control plane knows about the app's remote cache setting.
type SpeedCacheState struct {
	Configured bool
	Enabled    bool
	TargetID   string
}

// SpeedFinding is one suggestion to make builds faster.
type SpeedFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	FixKind  string `json:"fix_kind"`
	TargetID string `json:"target_id,omitempty"`
}

// SpeedReport is the build speed summary and advice for one app.
type SpeedReport struct {
	Builds           int            `json:"builds"`
	WarmP50Seconds   float64        `json:"warm_p50_seconds"`
	ColdP50Seconds   float64        `json:"cold_p50_seconds"`
	WarmKnown        bool           `json:"warm_known"`
	TimeSavedSeconds float64        `json:"time_saved_seconds"`
	Findings         []SpeedFinding `json:"findings"`
}

// AdviseSpeed analyses runs (newest first) and returns time saved by cache and fix suggestions.
func AdviseSpeed(runs []Run, cache SpeedCacheState) SpeedReport {
	rep := SpeedReport{Builds: len(runs), Findings: []SpeedFinding{}}
	trend := ComputeTrend(runs)

	var warm, cold, all []float64
	for _, r := range runs {
		if r.Status != RunSuccess || r.Duration < 0 {
			continue
		}
		all = append(all, r.Duration.Seconds())
		if r.TotalSteps == 0 {
			continue
		}
		ratio := float64(r.CachedSteps) / float64(r.TotalSteps)
		switch {
		case ratio >= warmRatio:
			warm = append(warm, r.Duration.Seconds())
		case ratio <= coldRatio:
			cold = append(cold, r.Duration.Seconds())
		}
	}
	if len(warm) > 0 && len(cold) > 0 {
		sort.Float64s(warm)
		sort.Float64s(cold)
		rep.WarmP50Seconds, rep.ColdP50Seconds = percentile(warm, 0.5), percentile(cold, 0.5)
		rep.WarmKnown = true
		if d := rep.ColdP50Seconds - rep.WarmP50Seconds; d > 0 {
			rep.TimeSavedSeconds = d * float64(len(warm))
		}
	}

	if cache.Configured && !cache.Enabled {
		rep.Findings = append(rep.Findings, SpeedFinding{Code: SpeedCacheDisabled, Severity: "warning", FixKind: SpeedFixAuto, TargetID: cache.TargetID})
	}
	if trend.CacheKnown && trend.Succeeded >= minRunsForCacheAdvice && trend.CacheHitRatio < lowCacheHitRatio {
		if !cache.Configured {
			rep.Findings = append(rep.Findings, SpeedFinding{Code: SpeedCacheUnconfigured, Severity: "warning", FixKind: SpeedFixGuided})
		}
		rep.Findings = append(rep.Findings, SpeedFinding{Code: SpeedLowCacheHit, Severity: "info", FixKind: SpeedFixManual})
	}
	if len(all) >= minRunsForRegression {
		half := len(all) / 2
		recent := append([]float64(nil), all[:half]...)
		older := append([]float64(nil), all[half:]...)
		sort.Float64s(recent)
		sort.Float64s(older)
		if o := percentile(older, 0.5); o > 0 && percentile(recent, 0.5) >= o*regressionFactor {
			rep.Findings = append(rep.Findings, SpeedFinding{Code: SpeedSlowerRecently, Severity: "info", FixKind: SpeedFixManual})
		}
	}
	return rep
}
