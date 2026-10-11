package usagecost

import (
	"sort"
	"time"
)

// Sample is one timestamped metric reading.
type Sample struct {
	At    time.Time
	Value float64
}

// defaultSampleInterval is the collector's tick, used when a window holds
// too few samples to infer one.
const defaultSampleInterval = 15 * time.Second

// Integrated is a metric reduced over one window.
type Integrated struct {
	Avg float64
	P50 float64
	// UpSeconds is how long the metric was actually reported, so downtime
	// counts as zero rather than as the average of the uptime.
	UpSeconds float64
}

// Integrate reduces samples inside [from, to). Each sample covers the gap to
// the next one, capped at twice the typical interval so a stopped container
// does not keep accruing.
func Integrate(samples []Sample, from, to time.Time) Integrated {
	in := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if !s.At.Before(from) && s.At.Before(to) {
			in = append(in, s)
		}
	}
	if len(in) == 0 {
		return Integrated{}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].At.Before(in[j].At) })
	interval := typicalInterval(in)
	var up, weighted float64
	values := make([]float64, len(in))
	for i, s := range in {
		values[i] = s.Value
		span := interval
		if i+1 < len(in) {
			if gap := in[i+1].At.Sub(s.At); gap < 2*interval {
				span = gap
			} else {
				span = 2 * interval
			}
		}
		if end := s.At.Add(span); end.After(to) {
			span = to.Sub(s.At)
		}
		up += span.Seconds()
		weighted += s.Value * span.Seconds()
	}
	sort.Float64s(values)
	out := Integrated{UpSeconds: up, P50: values[len(values)/2]}
	if up > 0 {
		out.Avg = weighted / up
	}
	return out
}

func typicalInterval(sorted []Sample) time.Duration {
	if len(sorted) < 2 {
		return defaultSampleInterval
	}
	gaps := make([]time.Duration, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		if g := sorted[i].At.Sub(sorted[i-1].At); g > 0 {
			gaps = append(gaps, g)
		}
	}
	if len(gaps) == 0 {
		return defaultSampleInterval
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	return gaps[len(gaps)/2]
}

// SumSamples adds every value inside [from, to), for counter delta metrics.
func SumSamples(samples []Sample, from, to time.Time) float64 {
	var total float64
	for _, s := range samples {
		if !s.At.Before(from) && s.At.Before(to) {
			total += s.Value
		}
	}
	return total
}

// UsageFromSamples builds a resource's daily usage from its CPU percent and
// memory byte series. CPU percent is of one core, so 100 is one core.
func UsageFromSamples(base DailyUsage, cpuPercent, memBytes []Sample, from, to time.Time) DailyUsage {
	cpu := Integrate(cpuPercent, from, to)
	mem := Integrate(memBytes, from, to)
	base.UpSeconds = max(cpu.UpSeconds, mem.UpSeconds)
	base.CPUAvgCores = cpu.Avg / 100
	base.CPUP50Cores = cpu.P50 / 100
	base.MemAvgBytes = mem.Avg
	base.MemP50Bytes = mem.P50
	return base
}
