package resourceadvisor

import (
	"fmt"
	"math"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

// growthFinding is rule d: steady memory growth fitted over hourly means,
// with a time to exhaustion against the limit or the node's memory.
func growthFinding(r Resource, nodeMemory float64, th Thresholds, now time.Time) (Finding, bool) {
	mem := within(r.Memory, now.Add(-th.Window))
	hourly := buckets(mem, time.Hour, false)
	if coverage(mem, now) < th.MinData || len(hourly) < 24 {
		return Finding{}, false
	}
	ft, ok := linearFit(hourly)
	if !ok || ft.slopePerHour <= 0 || ft.r2 < th.GrowthMinFit {
		return Finding{}, false
	}
	startV, endV := ft.at(ft.start), ft.at(ft.end)
	if startV <= 0 || (endV-startV)/startV < th.GrowthMinFraction {
		return Finding{}, false
	}
	ceiling, against := float64(r.MemoryLimit), "its memory limit"
	if ceiling <= 0 && nodeMemory > 0 {
		ceiling, against = nodeMemory, "the server's memory"
	}
	perDay := ft.slopePerHour * 24
	f := newFinding(KindMemoryGrowth, SeverityInfo, r.Ref)
	f.Title = fmt.Sprintf("Memory of %s keeps growing", r.Ref.Name)
	f.Evidence = memoryEvidence(mem, r.MemoryLimit, th.Window)
	f.Evidence.Numbers = map[string]float64{"growth_bytes_per_day": perDay, "fit_r2": math.Round(ft.r2*100) / 100}
	f.Params["per_day"] = rightsizing.FormatMemory(int64(perDay))
	f.Params["window"] = f.Evidence.Window
	why := fmt.Sprintf("Memory of %s rose from about %s to %s over the %s, roughly %s a day in a steady line (fit %.2f). That pattern usually means a leak or an unbounded cache.",
		r.Ref.Name, rightsizing.FormatMemory(int64(startV)), rightsizing.FormatMemory(int64(endV)), f.Evidence.Window, rightsizing.FormatMemory(int64(perDay)), ft.r2)
	if ceiling > endV {
		eta := now.Add(time.Duration((ceiling - endV) / ft.slopePerHour * float64(time.Hour)))
		f.Impact.ExhaustionAt = &eta
		left := eta.Sub(now)
		why += fmt.Sprintf(" At this rate it reaches %s in about %s.", against, rightsizing.HumanDuration(left))
		f.Params["eta"] = rightsizing.HumanDuration(left)
		f.Severity = SeveritySuggest
		if left < 7*day {
			f.Severity = SeverityWarn
		}
	}
	if r.Ref.Kind == ResourceDatabase {
		why += " Databases also grow while their cache warms after a restart; if the line flattens, ignore this."
		if f.Severity == SeverityWarn {
			f.Severity = SeveritySuggest
		}
	}
	f.Why = why
	f.Impact.Risk = "When growth meets the limit the container is OOM-killed; without a limit the whole server runs out of memory."
	f.Action = &Action{Type: ActionManual, Instructions: "Compare memory before and after the last deploys on the metrics page, and take a heap profile if the app supports one. A memory limit turns a server-wide outage into one restart."}
	return f, true
}
