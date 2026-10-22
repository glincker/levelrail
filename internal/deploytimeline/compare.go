package deploytimeline

import "sort"

// PhaseDelta lines up one phase of two deploys. A side is nil when that
// deploy never reached the phase.
type PhaseDelta struct {
	Phase   Phase  `json:"phase"`
	AMS     *int64 `json:"a_ms,omitempty"`
	BMS     *int64 `json:"b_ms,omitempty"`
	DeltaMS *int64 `json:"delta_ms,omitempty"`
}

// ImpactDelta is how the traffic shift differs between the two deploys.
type ImpactDelta struct {
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	P95Ms        float64 `json:"p95_ms"`
	RatePerSec   float64 `json:"rate_per_sec"`
}

// Comparison puts two timelines next to each other. Delta is B minus A.
type Comparison struct {
	A      Timeline     `json:"a"`
	B      Timeline     `json:"b"`
	Phases []PhaseDelta `json:"phases"`
	// OnlyA and OnlyB list event codes seen in one deploy and not the other,
	// which is where a crashloop or a rollback shows up.
	OnlyA  []string     `json:"only_a"`
	OnlyB  []string     `json:"only_b"`
	Impact *ImpactDelta `json:"impact,omitempty"`
}

func codeSet(events []Event) map[string]bool {
	set := map[string]bool{}
	for _, e := range events {
		if e.Kind == KindLog || e.Code == "readiness.probe" {
			continue
		}
		set[e.Code] = true
	}
	return set
}

func onlyIn(a, b map[string]bool) []string {
	out := []string{}
	for c := range a {
		if !b[c] {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

func impactChange(a, b *Impact) *ImpactDelta {
	if a == nil || b == nil {
		return nil
	}
	shift := func(im *Impact) (rate, errRate, p95 float64) {
		return im.After.RatePerSec - im.Before.RatePerSec, im.After.ErrorRate5xx - im.Before.ErrorRate5xx, im.After.P95Ms - im.Before.P95Ms
	}
	ar, ae, ap := shift(a)
	br, be, bp := shift(b)
	return &ImpactDelta{RatePerSec: br - ar, ErrorRate5xx: be - ae, P95Ms: bp - ap}
}

// Compare lines up two timelines by phase.
func Compare(a, b Timeline) Comparison {
	spans := func(t Timeline) map[Phase]int64 {
		m := map[Phase]int64{}
		for _, s := range t.Phases {
			m[s.Phase] = s.DurationMS
		}
		return m
	}
	am, bm := spans(a), spans(b)
	phases := []PhaseDelta{}
	for _, p := range phaseOrder {
		av, aok := am[p]
		bv, bok := bm[p]
		if !aok && !bok {
			continue
		}
		d := PhaseDelta{Phase: p}
		if aok {
			d.AMS = &av
		}
		if bok {
			d.BMS = &bv
		}
		if aok && bok {
			delta := bv - av
			d.DeltaMS = &delta
		}
		phases = append(phases, d)
	}
	ac, bc := codeSet(a.Events), codeSet(b.Events)
	return Comparison{
		A: a, B: b, Phases: phases, OnlyA: onlyIn(ac, bc), OnlyB: onlyIn(bc, ac), Impact: impactChange(a.Impact, b.Impact),
	}
}
