package migrateverify

import (
	"context"
	"time"
)

// Build compares the snapshots, optionally probes every source domain on both
// sides, and returns the finished report. A nil prober skips the probes.
func Build(ctx context.Context, src Source, tgt Target, prober *Prober, th Thresholds, now time.Time) Report {
	cmp := Compare(src, tgt)
	rep := Report{Platform: src.Platform, GeneratedAt: now.UTC(), Differences: cmp.Differences, Categories: cmp.Totals}
	if prober == nil {
		rep.Notes = append(rep.Notes, "live probes were skipped")
		rep.Finish()
		return rep
	}

	var pairs []Pair
	for _, m := range cmp.Matched {
		for _, d := range m.Source.Domains {
			if !ValidDomain(d) {
				rep.Differences = append(rep.Differences, Difference{Category: CatProbes, Severity: Info, App: m.Source.Name, Subject: d,
					Detail: "not probed: not a plain hostname", Fix: "check this domain by hand"})
				continue
			}
			pairs = append(pairs, Pair{App: m.Source.Name, Domain: d, Staged: m.Target.Staged})
		}
	}
	rep.Probes = prober.ProbeAll(ctx, pairs)
	rep.Differences = append(rep.Differences, CompareProbes(rep.Probes, th)...)
	for i := range rep.Categories {
		if rep.Categories[i].Category != CatProbes {
			continue
		}
		rep.Categories[i].Source = len(rep.Probes)
		rep.Categories[i].Target = len(rep.Probes)
		for _, p := range rep.Probes {
			if p.Source.Error == "" && p.Target.Error == "" && p.Source.Status == p.Target.Status {
				rep.Categories[i].Matched++
			}
		}
	}
	if prober.Opts.TargetAddr == "" {
		rep.Notes = append(rep.Notes, "this instance was probed through public DNS: pass a target address to probe the ingress directly before DNS moves")
	}
	rep.Finish()
	return rep
}
