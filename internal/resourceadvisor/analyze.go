package resourceadvisor

import (
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

// Analyze runs every rule. The same Input always yields the same findings,
// ordered by severity, then saving, then id.
func Analyze(in Input) []Finding {
	th := in.Thresholds
	if th.Window <= 0 {
		th = DefaultThresholds()
	}
	now := in.Now
	nodeMem := map[string]float64{}
	for _, n := range in.Nodes {
		nodeMem[n.ID] = n.MemoryTotal
	}
	var out []Finding
	byNode := map[string][]Resource{}
	for _, r := range in.Resources {
		byNode[r.NodeID] = append(byNode[r.NodeID], r)
		if r.Suspended {
			continue
		}
		if c := coverage(r.Memory, now); c < th.MinData {
			out = append(out, notEnoughData(r, c, th))
			continue
		}
		out = append(out, limitFindings(r, th, now)...)
		if f, ok := growthFinding(r, nodeMem[r.NodeID], th, now); ok {
			out = append(out, f)
		}
		if f, ok := idleFinding(r, th, now); ok {
			out = append(out, f)
		}
		if f, ok := tuningFinding(r, th, now); ok {
			out = append(out, f)
		}
	}
	for _, n := range in.Nodes {
		res := byNode[n.ID]
		var dbs []Resource
		for _, r := range res {
			if r.Ref.Kind == ResourceDatabase {
				dbs = append(dbs, r)
			}
		}
		out = append(out, consolidateFinding(n, dbs, th, now)...)
		out = append(out, diskFindings(n, th, now)...)
		out = append(out, hostFindings(n, res, th, now)...)
	}
	Sort(out)
	return out
}

func notEnoughData(r Resource, have time.Duration, th Thresholds) Finding {
	f := newFinding(KindNotEnoughData, SeverityInfo, r.Ref)
	f.Title = fmt.Sprintf("Not enough data yet for %s", r.Ref.Name)
	f.Why = fmt.Sprintf("The advisor needs at least %s of usage history before it suggests anything, and has %s so far.", rightsizing.HumanDuration(th.MinData), rightsizing.HumanDuration(have))
	f.Evidence = Evidence{Window: rightsizing.HumanDuration(have), WindowSeconds: int64(have.Seconds()), Samples: len(r.Memory)}
	f.Params["need"], f.Params["have"] = rightsizing.HumanDuration(th.MinData), rightsizing.HumanDuration(have)
	return f
}

// Sort orders findings by severity, then estimated memory saving, then id.
func Sort(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Severity.Rank() != b.Severity.Rank() {
			return a.Severity.Rank() > b.Severity.Rank()
		}
		if a.Impact.SavingMemoryBytes != b.Impact.SavingMemoryBytes {
			return a.Impact.SavingMemoryBytes > b.Impact.SavingMemoryBytes
		}
		return a.ID < b.ID
	})
}

// Summary counts findings for the dashboard tile and the attention feed.
type Summary struct {
	Total             int            `json:"total"`
	BySeverity        map[string]int `json:"by_severity"`
	ByKind            map[string]int `json:"by_kind"`
	Actionable        int            `json:"actionable"`
	PendingData       int            `json:"pending_data"`
	SavingMemoryBytes int64          `json:"saving_memory_bytes"`
	SavingDiskBytes   int64          `json:"saving_disk_bytes"`
	TopSavings        []Finding      `json:"top_savings"`
}

// Summarize builds a Summary; not-enough-data findings are counted apart.
func Summarize(fs []Finding, top int) Summary {
	s := Summary{BySeverity: map[string]int{}, ByKind: map[string]int{}, TopSavings: []Finding{}}
	var saving []Finding
	for _, f := range fs {
		if f.Kind == KindNotEnoughData {
			s.PendingData++
			continue
		}
		s.Total++
		s.BySeverity[string(f.Severity)]++
		s.ByKind[string(f.Kind)]++
		if f.Action != nil && f.Action.Appliable() {
			s.Actionable++
		}
		s.SavingMemoryBytes += f.Impact.SavingMemoryBytes
		s.SavingDiskBytes += f.Impact.SavingDiskBytes
		if f.Impact.SavingMemoryBytes > 0 || f.Impact.SavingDiskBytes > 0 {
			saving = append(saving, f)
		}
	}
	sort.SliceStable(saving, func(i, j int) bool {
		a := saving[i].Impact.SavingMemoryBytes + saving[i].Impact.SavingDiskBytes
		b := saving[j].Impact.SavingMemoryBytes + saving[j].Impact.SavingDiskBytes
		return a > b
	})
	if len(saving) > top {
		saving = saving[:top]
	}
	for _, f := range saving {
		f.Evidence.Series = nil
		s.TopSavings = append(s.TopSavings, f)
	}
	return s
}
