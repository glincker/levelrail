package resourceadvisor

import (
	"math/rand"
	"time"
)

var testNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

// series builds samples every step across span ending at testNow. noise is
// a +/- fraction applied with a fixed seed so tests stay deterministic.
func series(span, step time.Duration, noise float64, fn func(frac float64) float64) []Sample {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic test noise
	var out []Sample
	n := int(span / step)
	for i := 0; i <= n; i++ {
		at := testNow.Add(-span + time.Duration(i)*step)
		v := fn(float64(i) / float64(n))
		if noise > 0 {
			v *= 1 + noise*(2*rng.Float64()-1)
		}
		out = append(out, Sample{At: at, Value: v})
	}
	return out
}

func flat(v float64) func(float64) float64 { return func(float64) float64 { return v } }

func app(name string) Ref { return Ref{Kind: ResourceApp, Name: name} }
func db(name string) Ref  { return Ref{Kind: ResourceDatabase, Name: name} }

func kinds(fs []Finding) map[Kind]Finding {
	out := map[Kind]Finding{}
	for _, f := range fs {
		out[f.Kind] = f
	}
	return out
}

const week = 7 * day
