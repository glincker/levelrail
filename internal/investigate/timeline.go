// Package investigate merges the signals that explain a spike (deploys,
// config changes, restarts, resource saturation) into one ordered timeline.
package investigate

import (
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Kinds added on top of changes.Kind.
const (
	KindRestart    = "restart"
	KindSaturation = "saturation"
)

// Severity of a timeline event.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Saturation env vars and defaults.
const (
	EnvSaturationCPU    = "APP_SATURATION_CPU_PERCENT"
	EnvSaturationMemory = "APP_SATURATION_MEMORY_PERCENT"
	EnvSaturationMinGap = "APP_SATURATION_MIN_SAMPLES"

	defaultSaturationCPU    = 90.0
	defaultSaturationMemory = 90.0
	defaultSaturationMin    = 2
)

// Event is one entry of the What changed timeline.
type Event struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	LikelyCause bool      `json:"likely_cause,omitempty"`
}

// Thresholds are the saturation limits in percent.
type Thresholds struct {
	CPUPercent    float64
	MemoryPercent float64
	MinSamples    int
}

// ThresholdsFromEnv reads the saturation limits from the environment.
func ThresholdsFromEnv() Thresholds {
	t := Thresholds{CPUPercent: defaultSaturationCPU, MemoryPercent: defaultSaturationMemory, MinSamples: defaultSaturationMin}
	if v, err := strconv.ParseFloat(os.Getenv(EnvSaturationCPU), 64); err == nil && v > 0 {
		t.CPUPercent = v
	}
	if v, err := strconv.ParseFloat(os.Getenv(EnvSaturationMemory), 64); err == nil && v > 0 {
		t.MemoryPercent = v
	}
	if v, err := strconv.Atoi(os.Getenv(EnvSaturationMinGap)); err == nil && v > 0 {
		t.MinSamples = v
	}
	return t
}

// FromChanges converts aggregator output into timeline events.
func FromChanges(cs []changes.Change) []Event {
	out := make([]Event, 0, len(cs))
	for _, c := range cs {
		sev := SeverityInfo
		if c.Kind == changes.KindRollback {
			sev = SeverityWarning
		}
		out = append(out, Event{
			At: c.At, Kind: string(c.Kind), Severity: sev, Title: c.Title,
			Detail: c.Detail, Ref: c.Ref, LikelyCause: c.LikelyCause,
		})
	}
	return out
}

// RestartEvents turns restart samples into events. Restarts closer together
// than gap collapse into one event whose title carries the count.
func RestartEvents(samples []telemetry.Sample, gap time.Duration) []Event {
	if len(samples) == 0 {
		return nil
	}
	sorted := append([]telemetry.Sample(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp.Before(sorted[j].Timestamp) })
	var out []Event
	start, last, n := sorted[0].Timestamp, sorted[0].Timestamp, 1
	flush := func() {
		title := "Container restarted"
		sev := SeverityWarning
		if n > 1 {
			title = "Container restarted " + strconv.Itoa(n) + " times"
		}
		if n >= 3 {
			sev = SeverityCritical
		}
		out = append(out, Event{At: start, Kind: KindRestart, Severity: sev, Title: title})
	}
	for _, s := range sorted[1:] {
		if s.Timestamp.Sub(last) <= gap {
			last = s.Timestamp
			n++
			continue
		}
		flush()
		start, last, n = s.Timestamp, s.Timestamp, 1
	}
	flush()
	return out
}

type run struct {
	start, end time.Time
	peak       float64
	n          int
}

func runsAbove(samples []telemetry.Sample, limit float64, value func(i int) float64) []run {
	var out []run
	var cur *run
	for i, s := range samples {
		v := value(i)
		if v >= limit {
			if cur == nil {
				cur = &run{start: s.Timestamp}
			}
			cur.end = s.Timestamp
			cur.n++
			if v > cur.peak {
				cur.peak = v
			}
			continue
		}
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func pct(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "%" }

// SaturationEvents finds sustained CPU and memory pressure. cpu is a percent
// series; memUsage and memLimit are bytes series sampled at the same times
// (a missing or zero limit skips memory).
func SaturationEvents(cpu, memUsage, memLimit []telemetry.Sample, th Thresholds) []Event {
	var out []Event
	for _, r := range runsAbove(cpu, th.CPUPercent, func(i int) float64 { return cpu[i].Value }) {
		if r.n < th.MinSamples {
			continue
		}
		out = append(out, Event{
			At: r.start, Kind: KindSaturation, Severity: SeverityWarning,
			Title:  "CPU at " + pct(r.peak) + " of its limit",
			Detail: "sustained from " + r.start.UTC().Format(time.RFC3339) + " to " + r.end.UTC().Format(time.RFC3339),
		})
	}
	if len(memUsage) > 0 && len(memLimit) > 0 {
		limits := make(map[int64]float64, len(memLimit))
		for _, s := range memLimit {
			limits[s.Timestamp.Unix()] = s.Value
		}
		ratio := func(i int) float64 {
			l := limits[memUsage[i].Timestamp.Unix()]
			if l <= 0 {
				return 0
			}
			return memUsage[i].Value / l * 100
		}
		for _, r := range runsAbove(memUsage, th.MemoryPercent, ratio) {
			if r.n < th.MinSamples {
				continue
			}
			out = append(out, Event{
				At: r.start, Kind: KindSaturation, Severity: SeverityCritical,
				Title:  "Memory at " + pct(r.peak) + " of its limit",
				Detail: "sustained from " + r.start.UTC().Format(time.RFC3339) + " to " + r.end.UTC().Format(time.RFC3339),
			})
		}
	}
	return out
}

var kindRank = map[string]int{
	string(changes.KindDeploy): 0, string(changes.KindRollback): 1, string(changes.KindConfig): 2,
	string(changes.KindEnv): 3, string(changes.KindSecret): 4, string(changes.KindDomain): 5,
	string(changes.KindScale): 6, string(changes.KindLB): 7, KindSaturation: 8, KindRestart: 9,
}

// Merge combines event lists into one timeline, oldest first. Ties order by
// kind rank (causes before symptoms) then title, so output is deterministic.
func Merge(lists ...[]Event) []Event {
	var all []Event
	for _, l := range lists {
		all = append(all, l...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].At.Equal(all[j].At) {
			return all[i].At.Before(all[j].At)
		}
		ri, rj := rank(all[i].Kind), rank(all[j].Kind)
		if ri != rj {
			return ri < rj
		}
		return all[i].Title < all[j].Title
	})
	if all == nil {
		return []Event{}
	}
	return all
}

func rank(kind string) int {
	if r, ok := kindRank[kind]; ok {
		return r
	}
	return len(kindRank)
}

// Window clamps events to [from, to].
func Window(events []Event, from, to time.Time) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if e.At.Before(from) || e.At.After(to) {
			continue
		}
		out = append(out, e)
	}
	return out
}
