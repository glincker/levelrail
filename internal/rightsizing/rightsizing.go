// Package rightsizing is a deterministic, pure-Go engine that turns an
// app's historical CPU/memory usage into a resource-limit suggestion.
// It is a read-and-suggest layer on top of the platform API, per this
// project's AI design rule: it never calls an external
// model, never writes to any resource, and its output is never applied
// automatically. Every recommendation is backed by literal samples or a
// literal OOM signal it was given; when there isn't enough history yet,
// Recommend says so rather than guessing.
package rightsizing

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

// Confidence levels a DimensionRecommendation can carry.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Actions a DimensionRecommendation can suggest.
const (
	ActionRaise = "raise"
	ActionLower = "lower"
	ActionKeep  = "keep"
)

const (
	// memorySafetyMargin/cpuSafetyMargin: a suggested limit is p95 usage
	// times this margin, headroom above the busiest 5% of observed
	// samples rather than the p95 value itself.
	memorySafetyMargin = 1.3
	cpuSafetyMargin    = 1.3

	// raiseUtilizationThreshold/lowerUtilizationThreshold: p95 usage as a
	// fraction of the current limit that triggers a raise or lower
	// suggestion; between the two the current limit is left alone.
	raiseUtilizationThreshold = 0.85
	lowerUtilizationThreshold = 0.40

	// oomLimitFloorMultiplier: after an OOM kill, the suggested limit
	// never falls below the prior limit times this factor. A container
	// was killed for exceeding its limit, so the p95 usage leading up to
	// that point understates the actual peak that caused the kill.
	oomLimitFloorMultiplier = 1.5

	// minSampleCount/minCoverage gate whether there is enough history to
	// suggest a change at all: too few samples, or samples spanning too
	// short a window, describe a brand-new app more than they describe
	// its steady-state usage.
	minSampleCount = 20
	minCoverage    = 6 * time.Hour

	mib      = 1024 * 1024
	nanoCore = 1_000_000_000
)

// Suggested limits round up to these steps so an operator never sees
// "0 cores" or "37 MiB". Above the last step they grow by the step size.
var (
	memoryStepsMiB = []int64{64, 96, 128, 192, 256, 384, 512, 768, 1024, 1536, 2048}
	cpuStepsMilli  = []int64{100, 250, 500, 750, 1000, 1500, 2000, 3000, 4000}
)

// Sample is one raw (timestamp, value) reading, the shape
// internal/telemetry.Sample already has trimmed to what this package
// needs, kept local so this package doesn't depend on internal/telemetry.
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// OOMEvidence is a genuinely observed OOM-kill signal (a log line
// matching a known OOM pattern; see internal/diagnose.OOMLogPatterns),
// never fabricated from usage numbers alone.
type OOMEvidence struct {
	DetectedAt time.Time
	Excerpt    string
}

// Input bundles one app's usage history and current limits. CPUPercentSamples
// is Docker's own 0-100-per-core convention (internal/docker.ContainerStats.
// CPUPercent's own doc comment): a container fully using 2 cores reads 200,
// converted internally to nano-CPUs to compare against CurrentNanoCPUs.
type Input struct {
	ServiceName        string
	Now                time.Time
	LookbackWindow     time.Duration
	MemorySamples      []Sample
	CPUPercentSamples  []Sample
	CurrentMemoryBytes int64
	CurrentNanoCPUs    int64
	OOM                *OOMEvidence
	// Tuning overrides the default thresholds; a zero field keeps its default.
	Tuning Tuning
}

// Tuning holds the thresholds Recommend uses, so callers can source them from env.
type Tuning struct {
	MemoryMargin float64
	CPUMargin    float64
	RaiseAt      float64
	LowerAt      float64
	MinCoverage  time.Duration
}

func (t Tuning) withDefaults() Tuning {
	if t.MemoryMargin <= 1 {
		t.MemoryMargin = memorySafetyMargin
	}
	if t.CPUMargin <= 1 {
		t.CPUMargin = cpuSafetyMargin
	}
	if t.RaiseAt <= 0 || t.RaiseAt > 1 {
		t.RaiseAt = raiseUtilizationThreshold
	}
	if t.LowerAt <= 0 || t.LowerAt >= t.RaiseAt {
		t.LowerAt = lowerUtilizationThreshold
	}
	if t.MinCoverage <= 0 {
		t.MinCoverage = minCoverage
	}
	return t
}

// DimensionRecommendation is one resource dimension's (memory or CPU)
// suggestion.
type DimensionRecommendation struct {
	Dimension      string
	SampleCount    int
	DataSufficient bool
	Confidence     string
	CurrentLimit   int64
	P95Usage       float64
	P99Usage       float64
	SuggestedLimit int64
	// Action is "" when there isn't enough data, or no limit is
	// currently set, to responsibly suggest raising or lowering one.
	Action string
	Reason string
}

// Result is Recommend's output: one recommendation per dimension, plus
// whatever OOM evidence fed into the memory one.
type Result struct {
	ServiceName    string
	LookbackWindow time.Duration
	Memory         DimensionRecommendation
	CPU            DimensionRecommendation
	OOM            *OOMEvidence
}

// Recommend synthesizes in's usage history and current limits into a
// deterministic suggestion per dimension. The same Input always produces
// the same Result.
func Recommend(in Input) Result {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	tun := in.Tuning.withDefaults()

	cpuNanoSamples := make([]Sample, len(in.CPUPercentSamples))
	for i, s := range in.CPUPercentSamples {
		cpuNanoSamples[i] = Sample{Timestamp: s.Timestamp, Value: s.Value / 100 * 1_000_000_000}
	}

	return Result{
		ServiceName:    in.ServiceName,
		LookbackWindow: in.LookbackWindow,
		Memory: recommendDimension(dimensionParams{
			dimension:    "memory",
			samples:      in.MemorySamples,
			currentLimit: in.CurrentMemoryBytes,
			now:          now,
			lookback:     in.LookbackWindow,
			margin:       tun.MemoryMargin,
			tuning:       tun,
			round:        RoundMemoryUp,
			format:       formatMemory,
			oom:          in.OOM,
		}),
		CPU: recommendDimension(dimensionParams{
			dimension:    "cpu",
			samples:      cpuNanoSamples,
			currentLimit: in.CurrentNanoCPUs,
			now:          now,
			lookback:     in.LookbackWindow,
			margin:       tun.CPUMargin,
			tuning:       tun,
			round:        RoundCPUUp,
			format:       formatCPU,
			oom:          nil, // an OOM kill is a memory signal, not a CPU one
		}),
		OOM: in.OOM,
	}
}

type dimensionParams struct {
	dimension    string
	samples      []Sample
	currentLimit int64
	now          time.Time
	lookback     time.Duration
	margin       float64
	tuning       Tuning
	round        func(float64) int64
	format       func(int64) string
	oom          *OOMEvidence
}

func recommendDimension(p dimensionParams) DimensionRecommendation {
	rec := DimensionRecommendation{
		Dimension:    p.dimension,
		SampleCount:  len(p.samples),
		CurrentLimit: p.currentLimit,
	}

	if len(p.samples) > 0 {
		values := make([]float64, len(p.samples))
		for i, s := range p.samples {
			values[i] = s.Value
		}
		rec.P95Usage = percentile(values, 0.95)
		rec.P99Usage = percentile(values, 0.99)
	}

	coverage := dataCoverage(p.samples, p.now)
	rec.Confidence = confidenceFor(len(p.samples), coverage, p.lookback, p.tuning.MinCoverage)
	rec.DataSufficient = rec.Confidence != ConfidenceLow

	if p.oom != nil {
		rec.Confidence = ConfidenceHigh
		rec.Action = ActionRaise
		suggested := p.round(rec.P95Usage * p.margin)
		if p.currentLimit > 0 {
			floor := p.round(float64(p.currentLimit) * oomLimitFloorMultiplier)
			if suggested < floor {
				suggested = floor
			}
		}
		rec.SuggestedLimit = suggested
		rec.Reason = fmt.Sprintf(
			"This app was OOM-killed on %s. The current limit is very likely too low regardless of average usage; consider raising it to at least %s.",
			p.oom.DetectedAt.UTC().Format(time.RFC3339), p.format(suggested),
		)
		return rec
	}

	if len(p.samples) == 0 {
		rec.Reason = fmt.Sprintf("No %s usage has been collected for this app yet. Check back once it has been running long enough to build up history.", p.dimension)
		return rec
	}

	rec.SuggestedLimit = p.round(rec.P95Usage * p.margin)

	if p.currentLimit <= 0 {
		rec.Reason = fmt.Sprintf(
			"No %s limit is currently configured. Based on p95 usage of %s over the %s, a limit around %s would leave headroom above typical peaks.",
			p.dimension, p.format(int64(rec.P95Usage)), HumanWindow(p.lookback), p.format(rec.SuggestedLimit),
		)
		return rec
	}

	if !rec.DataSufficient {
		rec.Reason = fmt.Sprintf(
			"Only %s of history is available so far (need at least %s); this app hasn't been running long enough for a confident recommendation yet.",
			HumanDuration(coverage), HumanDuration(p.tuning.MinCoverage),
		)
		return rec
	}

	utilization := rec.P95Usage / float64(p.currentLimit)
	window := HumanWindow(p.lookback)
	switch {
	case utilization >= p.tuning.RaiseAt:
		rec.Action = ActionRaise
		rec.Reason = fmt.Sprintf(
			"p95 usage over the %s is %s, about %.0f%% of the current %s limit. Consider raising it to roughly %s.",
			window, p.format(int64(rec.P95Usage)), utilization*100, p.dimension, p.format(rec.SuggestedLimit),
		)
	case utilization <= p.tuning.LowerAt && rec.SuggestedLimit < p.currentLimit:
		rec.Action = ActionLower
		rec.Reason = fmt.Sprintf(
			"p95 usage over the %s is %s, only about %.0f%% of the current %s limit. Consider lowering it to roughly %s to reclaim capacity.",
			window, p.format(int64(rec.P95Usage)), utilization*100, p.dimension, p.format(rec.SuggestedLimit),
		)
	default:
		rec.Action = ActionKeep
		rec.Reason = fmt.Sprintf(
			"p95 usage over the %s is %s, about %.0f%% of the current %s limit. That's a comfortable range; no change suggested.",
			window, p.format(int64(rec.P95Usage)), utilization*100, p.dimension,
		)
	}
	return rec
}

// dataCoverage is how long ago the earliest sample was taken, relative to
// now: a brand-new app has a short coverage span even if LookbackWindow
// itself is 7 days.
func dataCoverage(samples []Sample, now time.Time) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	earliest := samples[0].Timestamp
	for _, s := range samples[1:] {
		if s.Timestamp.Before(earliest) {
			earliest = s.Timestamp
		}
	}
	return now.Sub(earliest)
}

func confidenceFor(sampleCount int, coverage, lookback, needCoverage time.Duration) string {
	switch {
	case sampleCount < minSampleCount || coverage < needCoverage:
		return ConfidenceLow
	case lookback > 0 && coverage < lookback:
		return ConfidenceMedium
	default:
		return ConfidenceHigh
	}
}

// percentile uses the nearest-rank method over a defensive copy of
// values, sorted ascending; p is a fraction in [0, 1].
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// RoundMemoryUp rounds a byte count up to the next suggested memory step.
func RoundMemoryUp(bytes float64) int64 {
	if bytes <= 0 {
		return 0
	}
	return roundUpToSteps(bytes/mib, memoryStepsMiB) * mib
}

// RoundCPUUp rounds nano-CPUs up to the next suggested step, never below 0.1 core.
func RoundCPUUp(nanoCPUs float64) int64 {
	if nanoCPUs <= 0 {
		return 0
	}
	return roundUpToSteps(nanoCPUs/1_000_000, cpuStepsMilli) * 1_000_000
}

func roundUpToSteps(v float64, steps []int64) int64 {
	for _, s := range steps {
		if v <= float64(s) {
			return s
		}
	}
	last := steps[len(steps)-1]
	step := last - steps[len(steps)-2]
	n := int64(math.Ceil((v - float64(last)) / float64(step)))
	return last + n*step
}

// HumanWindow renders a lookback as "last 7 days" rather than "168h0m0s".
func HumanWindow(d time.Duration) string {
	if d <= 0 {
		return "observed history"
	}
	return "last " + HumanDuration(d)
}

// HumanDuration renders a duration in whole days, hours or minutes.
func HumanDuration(d time.Duration) string {
	unit := func(n int64, one string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %ss", n, one)
	}
	switch {
	case d >= 48*time.Hour:
		return unit(int64(d/(24*time.Hour)), "day")
	case d >= 2*time.Hour:
		return unit(int64(d/time.Hour), "hour")
	default:
		return unit(int64(d/time.Minute), "minute")
	}
}

// FormatMemory renders bytes as MiB or GiB.
func FormatMemory(bytes int64) string { return formatMemory(bytes) }

// FormatCPU renders nano-CPUs as cores.
func FormatCPU(nanoCPUs int64) string { return formatCPU(nanoCPUs) }

func formatMemory(bytes int64) string {
	if bytes <= 0 {
		return "0 MiB"
	}
	if bytes >= 1024*mib && bytes%(256*mib) == 0 {
		return strconv.FormatFloat(float64(bytes)/(1024*mib), 'f', -1, 64) + " GiB"
	}
	return fmt.Sprintf("%d MiB", (bytes+mib/2)/mib)
}

func formatCPU(nanoCPUs int64) string {
	if nanoCPUs <= 0 {
		return "0 cores"
	}
	v := float64(nanoCPUs) / nanoCore
	if v == 1 {
		return "1 core"
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) + " cores"
}
