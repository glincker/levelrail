package resourceadvisor

import (
	"fmt"
	"math"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

func toRightsizing(samples []Sample) []rightsizing.Sample {
	out := make([]rightsizing.Sample, len(samples))
	for i, s := range samples {
		out[i] = rightsizing.Sample{Timestamp: s.At, Value: s.Value}
	}
	return out
}

func memoryEvidence(samples []Sample, limit int64, window time.Duration) Evidence {
	s := summarize(samples)
	if held := coverage(samples, latest(samples)); held > 0 && held < window {
		window = held.Round(time.Hour)
	}
	return Evidence{
		Window: rightsizing.HumanWindow(window), WindowSeconds: int64(window.Seconds()), Samples: s.n,
		Unit: UnitBytes, P50: s.p50, P95: s.p95, P99: s.p99, Max: s.max, Limit: float64(limit),
		Series: evidenceSeries(samples),
	}
}

func cpuEvidence(nano []Sample, limit int64, window time.Duration) Evidence {
	e := memoryEvidence(nano, limit, window)
	e.Unit = UnitNanoCPUs
	return e
}

// limitFindings covers rules a, b and c: missing, tight and oversized limits.
func limitFindings(r Resource, th Thresholds, now time.Time) []Finding {
	since := now.Add(-th.Window)
	mem := within(r.Memory, since)
	cpu := cpuSamplesNano(within(r.CPUPercent, since))
	var oom *rightsizing.OOMEvidence
	if r.OOM != nil && !r.OOM.At.Before(since) {
		oom = &rightsizing.OOMEvidence{DetectedAt: r.OOM.At, Excerpt: r.OOM.Excerpt}
	}
	rec := rightsizing.Recommend(rightsizing.Input{
		ServiceName: r.Ref.Name, Now: now, LookbackWindow: th.Window,
		MemorySamples: toRightsizing(mem), CPUPercentSamples: toRightsizing(within(r.CPUPercent, since)),
		CurrentMemoryBytes: r.MemoryLimit, CurrentNanoCPUs: r.NanoCPUs, OOM: oom,
		Tuning: rightsizing.Tuning{
			MemoryMargin: th.MemoryHeadroom, CPUMargin: th.CPUHeadroom,
			RaiseAt: th.TightRatio, LowerAt: th.OverProvisionRatio, MinCoverage: th.MinData,
		},
	})
	fullWindow := coverage(mem, now) >= th.Window*95/100
	var out []Finding
	if f, ok := memoryLimitFinding(r, rec.Memory, mem, oom, th, fullWindow); ok {
		out = append(out, f)
	}
	if f, ok := cpuLimitFinding(r, rec.CPU, cpu, th, fullWindow); ok {
		out = append(out, f)
	}
	return out
}

func memoryLimitFinding(r Resource, rec rightsizing.DimensionRecommendation, mem []Sample, oom *rightsizing.OOMEvidence, th Thresholds, fullWindow bool) (Finding, bool) {
	if len(mem) == 0 {
		return Finding{}, false
	}
	ev := memoryEvidence(mem, r.MemoryLimit, th.Window)
	switch {
	case r.MemoryLimit <= 0:
		value := rightsizing.RoundMemoryUp(ev.P95 * th.MemoryHeadroom)
		value = max(value, rightsizing.RoundMemoryUp(ev.Max))
		f := newFinding(KindMemoryLimitMissing, SeveritySuggest, r.Ref)
		f.Title = fmt.Sprintf("Set a memory limit on %s", r.Ref.Name)
		f.Why = fmt.Sprintf("%s has no memory limit, so a leak or a spike can take memory from every other container on the server. Over the %s its p95 was %s and its peak %s; a %s limit keeps %.0f%% headroom above p95 while capping the blast radius.",
			r.Ref.Name, ev.Window, rightsizing.FormatMemory(int64(ev.P95)), rightsizing.FormatMemory(int64(ev.Max)), rightsizing.FormatMemory(value), (float64(value)/math.Max(ev.P95, 1)-1)*100)
		f.Evidence = ev
		f.Impact.Risk = "Without a limit the kernel picks the OOM victim, which can be another app, a database or the control plane."
		f.Action = &Action{Type: ActionSetMemoryLimit, Value: value}
		f.Params["p95"], f.Params["peak"], f.Params["value"] = rightsizing.FormatMemory(int64(ev.P95)), rightsizing.FormatMemory(int64(ev.Max)), rightsizing.FormatMemory(value)
		f.Params["window"] = ev.Window
		return f, true
	case oom != nil || rec.Action == rightsizing.ActionRaise || (r.Restarts >= th.RestartsTight && ev.P99 >= 0.8*float64(r.MemoryLimit)):
		value := rec.SuggestedLimit
		if value <= r.MemoryLimit {
			value = rightsizing.RoundMemoryUp(float64(r.MemoryLimit) * 1.25)
		}
		f := newFinding(KindMemoryLimitTight, SeverityWarn, r.Ref)
		f.Title = fmt.Sprintf("Memory limit of %s is too tight", r.Ref.Name)
		reason := fmt.Sprintf("p95 usage is %.0f%% of the %s limit", ev.P95/float64(r.MemoryLimit)*100, rightsizing.FormatMemory(r.MemoryLimit))
		if oom != nil {
			reason = "it was OOM-killed at " + oom.DetectedAt.UTC().Format(time.RFC3339)
		} else if r.Restarts >= th.RestartsTight {
			reason = fmt.Sprintf("it restarted %d times while running close to the %s limit", r.Restarts, rightsizing.FormatMemory(r.MemoryLimit))
		}
		f.Why = fmt.Sprintf("Over the %s %s. Raising the limit to %s gives it room before the kernel kills it.", ev.Window, reason, rightsizing.FormatMemory(value))
		f.Evidence = ev
		f.Evidence.Numbers = map[string]float64{"restarts": float64(r.Restarts)}
		f.Impact.Risk = "Running at the limit means OOM kills and restarts under load."
		f.Action = &Action{Type: ActionSetMemoryLimit, Value: value, Current: r.MemoryLimit}
		f.Params["reason"], f.Params["value"], f.Params["limit"] = reason, rightsizing.FormatMemory(value), rightsizing.FormatMemory(r.MemoryLimit)
		f.Params["window"] = ev.Window
		return f, true
	case fullWindow && ev.P99 <= th.OverProvisionRatio*float64(r.MemoryLimit):
		value := rightsizing.RoundMemoryUp(math.Max(ev.P99*th.MemoryHeadroom, ev.Max))
		if value >= r.MemoryLimit {
			return Finding{}, false
		}
		f := newFinding(KindMemoryOverProvisioned, SeveritySuggest, r.Ref)
		f.Title = fmt.Sprintf("Memory limit of %s is far above its use", r.Ref.Name)
		f.Why = fmt.Sprintf("For the whole %s p99 stayed at %s, %.0f%% of the %s limit. A %s limit still covers the peak of %s and returns %s to the server's budget.",
			ev.Window, rightsizing.FormatMemory(int64(ev.P99)), ev.P99/float64(r.MemoryLimit)*100, rightsizing.FormatMemory(r.MemoryLimit), rightsizing.FormatMemory(value), rightsizing.FormatMemory(int64(ev.Max)), rightsizing.FormatMemory(r.MemoryLimit-value))
		f.Evidence = ev
		f.Impact.SavingMemoryBytes = r.MemoryLimit - value
		f.Action = &Action{Type: ActionSetMemoryLimit, Value: value, Current: r.MemoryLimit}
		f.Params["p99"], f.Params["value"], f.Params["limit"] = rightsizing.FormatMemory(int64(ev.P99)), rightsizing.FormatMemory(value), rightsizing.FormatMemory(r.MemoryLimit)
		f.Params["window"] = ev.Window
		return f, true
	}
	return Finding{}, false
}

func cpuLimitFinding(r Resource, rec rightsizing.DimensionRecommendation, cpu []Sample, th Thresholds, fullWindow bool) (Finding, bool) {
	if len(cpu) == 0 {
		return Finding{}, false
	}
	ev := cpuEvidence(cpu, r.NanoCPUs, th.Window)
	switch {
	case r.NanoCPUs <= 0:
		value := rightsizing.RoundCPUUp(math.Max(ev.P99, ev.P95*th.CPUHeadroom))
		// Databases burst on checkpoints, vacuum and index builds, so a
		// cap sized from a quiet average would stall them.
		if r.Ref.Kind == ResourceDatabase {
			value = max(value, th.DatabaseCPUFloor)
		}
		f := newFinding(KindCPULimitMissing, SeverityInfo, r.Ref)
		f.Title = fmt.Sprintf("Set a CPU limit on %s", r.Ref.Name)
		f.Why = fmt.Sprintf("%s can use every core, so a busy loop starves its neighbours. Over the %s its p95 was %s and p99 %s; a %s cap covers that with room for bursts.",
			r.Ref.Name, ev.Window, rightsizing.FormatCPU(int64(ev.P95)), rightsizing.FormatCPU(int64(ev.P99)), rightsizing.FormatCPU(value))
		f.Evidence = ev
		f.Impact.Risk = "A CPU cap throttles bursts such as startup or cache warmup; watch latency after applying it."
		f.Action = &Action{Type: ActionSetCPULimit, Value: value}
		f.Params["p95"], f.Params["value"], f.Params["window"] = rightsizing.FormatCPU(int64(ev.P95)), rightsizing.FormatCPU(value), ev.Window
		return f, true
	case rec.Action == rightsizing.ActionRaise:
		value := rightsizing.RoundCPUUp(math.Max(float64(rec.SuggestedLimit), float64(r.NanoCPUs)*1.25))
		f := newFinding(KindCPULimitTight, SeverityWarn, r.Ref)
		f.Title = fmt.Sprintf("CPU limit of %s is throttling it", r.Ref.Name)
		f.Why = fmt.Sprintf("Over the %s p95 CPU was %s against a %s cap, so requests queue behind throttling. Raising the cap to %s removes that.",
			ev.Window, rightsizing.FormatCPU(int64(ev.P95)), rightsizing.FormatCPU(r.NanoCPUs), rightsizing.FormatCPU(value))
		f.Evidence = ev
		f.Action = &Action{Type: ActionSetCPULimit, Value: value, Current: r.NanoCPUs}
		f.Params["value"], f.Params["limit"], f.Params["window"] = rightsizing.FormatCPU(value), rightsizing.FormatCPU(r.NanoCPUs), ev.Window
		return f, true
	case fullWindow && ev.P99 <= th.OverProvisionRatio*float64(r.NanoCPUs):
		value := rightsizing.RoundCPUUp(math.Max(ev.P99*th.CPUHeadroom, ev.Max))
		if value >= r.NanoCPUs {
			return Finding{}, false
		}
		f := newFinding(KindCPUOverProvisioned, SeverityInfo, r.Ref)
		f.Title = fmt.Sprintf("CPU limit of %s is far above its use", r.Ref.Name)
		f.Why = fmt.Sprintf("For the whole %s p99 CPU stayed at %s under a %s cap. %s still covers the peak.",
			ev.Window, rightsizing.FormatCPU(int64(ev.P99)), rightsizing.FormatCPU(r.NanoCPUs), rightsizing.FormatCPU(value))
		f.Evidence = ev
		f.Impact.SavingNanoCPUs = r.NanoCPUs - value
		f.Action = &Action{Type: ActionSetCPULimit, Value: value, Current: r.NanoCPUs}
		f.Params["value"], f.Params["limit"], f.Params["window"] = rightsizing.FormatCPU(value), rightsizing.FormatCPU(r.NanoCPUs), ev.Window
		return f, true
	}
	return Finding{}, false
}
