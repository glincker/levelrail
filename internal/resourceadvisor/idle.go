package resourceadvisor

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

// idleFinding is rule e. Apps need a routed domain so a missing request
// sample really means no traffic; databases need a known zero app links.
func idleFinding(r Resource, th Thresholds, now time.Time) (Finding, bool) {
	if r.Suspended {
		return Finding{}, false
	}
	since := now.Add(-th.IdleWindow)
	mem := within(r.Memory, since)
	if coverage(mem, now) < th.IdleWindow*95/100 {
		return Finding{}, false
	}
	ms := summarize(mem)
	window := rightsizing.HumanWindow(th.IdleWindow)
	switch r.Ref.Kind {
	case ResourceApp:
		if !r.ServesTraffic || r.SleepEnabled {
			return Finding{}, false
		}
		total := 0.0
		for _, s := range within(r.Requests, since) {
			total += s.Value
		}
		if total > 0 {
			return Finding{}, false
		}
		f := newFinding(KindIdleApp, SeveritySuggest, r.Ref)
		f.Title = fmt.Sprintf("%s had no requests for the %s", r.Ref.Name, window)
		f.Why = fmt.Sprintf("No request reached %s through the proxy in the %s, yet it held about %s of memory. Sleep when idle stops it after %d quiet minutes and starts it again on the next request.",
			r.Ref.Name, window, rightsizing.FormatMemory(int64(ms.p95)), th.SleepIdleMinutes)
		f.Evidence = Evidence{Window: window, WindowSeconds: int64(th.IdleWindow.Seconds()), Samples: ms.n, Unit: UnitRequests, Numbers: map[string]float64{"requests": 0, "memory_p95_bytes": ms.p95}}
		f.Impact.SavingMemoryBytes = int64(ms.p95)
		f.Impact.Risk = "The first request after a sleep waits for the container to start."
		f.Action = &Action{Type: ActionEnableSleep, Value: th.SleepIdleMinutes}
		f.Params["window"], f.Params["memory"], f.Params["minutes"] = window, rightsizing.FormatMemory(int64(ms.p95)), fmt.Sprint(th.SleepIdleMinutes)
		return f, true
	case ResourceDatabase:
		if r.LinkedApps != 0 {
			return Finding{}, false
		}
		cpu := summarize(within(r.CPUPercent, since))
		if cpu.n == 0 || cpu.p99 >= th.IdleCPUPercent {
			return Finding{}, false
		}
		f := newFinding(KindIdleDatabase, SeveritySuggest, r.Ref)
		f.Title = fmt.Sprintf("%s looks unused", r.Ref.Name)
		f.Why = fmt.Sprintf("No app is linked to %s and its CPU stayed under %.1f%% (p99) for the %s while it held %s of memory. Connection counts are not measured, so confirm nothing outside this server uses it.",
			r.Ref.Name, th.IdleCPUPercent, window, rightsizing.FormatMemory(int64(ms.p95)))
		f.Evidence = Evidence{Window: window, WindowSeconds: int64(th.IdleWindow.Seconds()), Samples: cpu.n, Numbers: map[string]float64{"cpu_p99_percent": cpu.p99, "memory_p95_bytes": ms.p95, "linked_apps": 0}}
		f.Impact.SavingMemoryBytes = int64(ms.p95)
		f.Action = &Action{Type: ActionManual, Instructions: "Run a backup from the database's Backups tab, check it restores, then stop the database from its page. Starting it again restores it as it was."}
		f.Params["window"], f.Params["memory"] = window, rightsizing.FormatMemory(int64(ms.p95))
		return f, true
	}
	return Finding{}, false
}
