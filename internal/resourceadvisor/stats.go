package resourceadvisor

import (
	"math"
	"sort"
	"time"
)

// within returns the samples at or after since, in time order.
func within(samples []Sample, since time.Time) []Sample {
	out := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if !s.At.Before(since) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// coverage is how far back the oldest sample reaches from now.
func coverage(samples []Sample, now time.Time) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	oldest := samples[0].At
	for _, s := range samples[1:] {
		if s.At.Before(oldest) {
			oldest = s.At
		}
	}
	return now.Sub(oldest)
}

type summary struct {
	n                  int
	p50, p95, p99, max float64
	mean               float64
}

func summarize(samples []Sample) summary {
	if len(samples) == 0 {
		return summary{}
	}
	vals := make([]float64, len(samples))
	sum := 0.0
	for i, s := range samples {
		vals[i] = s.Value
		sum += s.Value
	}
	sort.Float64s(vals)
	return summary{
		n: len(vals), p50: rank(vals, 0.5), p95: rank(vals, 0.95), p99: rank(vals, 0.99),
		max: vals[len(vals)-1], mean: sum / float64(len(vals)),
	}
}

// rank is the nearest-rank percentile of sorted values.
func rank(sorted []float64, p float64) float64 {
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	idx = max(0, min(idx, len(sorted)-1))
	return sorted[idx]
}

// buckets averages samples into fixed-width buckets, oldest first; empty
// buckets are skipped so gaps never read as zero usage.
func buckets(samples []Sample, width time.Duration, peak bool) []Point {
	if len(samples) == 0 || width <= 0 {
		return nil
	}
	type acc struct {
		sum, max float64
		n        int
	}
	m := map[int64]*acc{}
	for _, s := range samples {
		k := s.At.Truncate(width).Unix()
		a := m[k]
		if a == nil {
			a = &acc{max: s.Value}
			m[k] = a
		}
		a.sum += s.Value
		a.n++
		a.max = math.Max(a.max, s.Value)
	}
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]Point, len(keys))
	for i, k := range keys {
		a := m[k]
		v := a.sum / float64(a.n)
		if peak {
			v = a.max
		}
		out[i] = Point{T: time.Unix(k, 0).UTC(), V: v}
	}
	return out
}

// evidenceSeries is an hourly peak series capped to 168 points.
func evidenceSeries(samples []Sample) []Point {
	width := time.Hour
	if c := coverage(samples, latest(samples)); c > 168*time.Hour {
		width = c / 168
	}
	return buckets(samples, width, true)
}

func latest(samples []Sample) time.Time {
	var t time.Time
	for _, s := range samples {
		if s.At.After(t) {
			t = s.At
		}
	}
	return t
}

type fit struct {
	slopePerHour float64
	intercept    float64
	r2           float64
	start, end   time.Time
}

func (f fit) at(t time.Time) float64 {
	return f.intercept + f.slopePerHour*t.Sub(f.start).Hours()
}

// linearFit is an ordinary least squares fit of value over hours.
func linearFit(points []Point) (fit, bool) {
	if len(points) < 3 {
		return fit{}, false
	}
	start := points[0].T
	var sx, sy, sxx, sxy float64
	n := float64(len(points))
	for _, p := range points {
		x := p.T.Sub(start).Hours()
		sx += x
		sy += p.V
		sxx += x * x
		sxy += x * p.V
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return fit{}, false
	}
	slope := (n*sxy - sx*sy) / den
	intercept := (sy - slope*sx) / n
	mean := sy / n
	var ssTot, ssRes float64
	for _, p := range points {
		x := p.T.Sub(start).Hours()
		ssTot += (p.V - mean) * (p.V - mean)
		r := p.V - (intercept + slope*x)
		ssRes += r * r
	}
	r2 := 0.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}
	return fit{slopePerHour: slope, intercept: intercept, r2: r2, start: start, end: points[len(points)-1].T}, true
}

func cpuNano(percent float64) float64 { return percent / 100 * 1e9 }

func cpuSamplesNano(samples []Sample) []Sample {
	out := make([]Sample, len(samples))
	for i, s := range samples {
		out[i] = Sample{At: s.At, Value: cpuNano(s.Value)}
	}
	return out
}
