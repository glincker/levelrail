// Package nodecapacity judges a node's disk, inode, memory and swap headroom against warn and critical thresholds and groups cleanup candidates by safety class.
package nodecapacity

import (
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Env names for the default thresholds, all percentages.
const (
	EnvDiskWarnFree  = "APP_NODE_DISK_WARN_FREE_PCT"
	EnvDiskCritFree  = "APP_NODE_DISK_CRIT_FREE_PCT"
	EnvInodeWarnFree = "APP_NODE_INODE_WARN_FREE_PCT"
	EnvInodeCritFree = "APP_NODE_INODE_CRIT_FREE_PCT"
	EnvMemWarnAvail  = "APP_NODE_MEM_WARN_AVAIL_PCT"
	EnvMemCritAvail  = "APP_NODE_MEM_CRIT_AVAIL_PCT"
	EnvSwapWarnUsed  = "APP_NODE_SWAP_WARN_USED_PCT" //nolint:gosec // env var name, not a credential
	EnvSwapCritUsed  = "APP_NODE_SWAP_CRIT_USED_PCT"
)

// Level is how close a metric is to its limit.
type Level string

// Levels, worst last.
const (
	LevelUnknown  Level = "unknown"
	LevelOK       Level = "ok"
	LevelWarn     Level = "warn"
	LevelCritical Level = "critical"
)

// Metric names.
const (
	MetricDisk   = "disk"
	MetricInodes = "inodes"
	MetricMemory = "memory"
	MetricSwap   = "swap"
)

// Band is a warn and critical pair. Free-style metrics (disk, inodes, memory) trip when the
// free percentage falls to or below the value; swap trips when the used percentage rises to it.
type Band struct {
	Warn     float64 `json:"warn"`
	Critical float64 `json:"critical"`
}

// Thresholds are the percentage bands for every watched metric.
type Thresholds struct {
	Disk   Band `json:"disk_free_pct"`
	Inodes Band `json:"inodes_free_pct"`
	Memory Band `json:"memory_available_pct"`
	Swap   Band `json:"swap_used_pct"`
}

// ThresholdsFromEnv reads the defaults, falling back on unset or invalid values.
func ThresholdsFromEnv(lookup func(string) (string, bool)) Thresholds {
	read := func(name string, def float64) float64 {
		if v, ok := lookup(name); ok {
			if n, err := strconv.ParseFloat(v, 64); err == nil && n >= 0 && n <= 100 {
				return n
			}
		}
		return def
	}
	t := Thresholds{
		Disk:   Band{Warn: read(EnvDiskWarnFree, 15), Critical: read(EnvDiskCritFree, 7)},
		Inodes: Band{Warn: read(EnvInodeWarnFree, 15), Critical: read(EnvInodeCritFree, 5)},
		Memory: Band{Warn: read(EnvMemWarnAvail, 10), Critical: read(EnvMemCritAvail, 4)},
		Swap:   Band{Warn: read(EnvSwapWarnUsed, 50), Critical: read(EnvSwapCritUsed, 85)},
	}
	return t.normalized()
}

// normalized keeps the critical band no looser than the warn band.
func (t Thresholds) normalized() Thresholds {
	for _, b := range []*Band{&t.Disk, &t.Inodes, &t.Memory} {
		b.Critical = min(b.Critical, b.Warn)
	}
	t.Swap.Critical = max(t.Swap.Critical, t.Swap.Warn)
	return t
}

// Merge applies a node's saved overrides on top of the defaults.
func (t Thresholds) Merge(g store.NodeGuardrails) Thresholds {
	set := func(dst *float64, v *float64) {
		if v != nil {
			*dst = *v
		}
	}
	set(&t.Disk.Warn, g.DiskWarnFreePct)
	set(&t.Disk.Critical, g.DiskCritFreePct)
	set(&t.Inodes.Warn, g.InodeWarnFreePct)
	set(&t.Inodes.Critical, g.InodeCritFreePct)
	set(&t.Memory.Warn, g.MemWarnAvailPct)
	set(&t.Memory.Critical, g.MemCritAvailPct)
	set(&t.Swap.Warn, g.SwapWarnUsedPct)
	set(&t.Swap.Critical, g.SwapCritUsedPct)
	return t.normalized()
}

// Validate rejects percentages outside 0..100 or a critical band looser than its warn band.
func (t Thresholds) Validate() bool {
	in := func(v float64) bool { return v >= 0 && v <= 100 }
	for _, b := range []Band{t.Disk, t.Inodes, t.Memory, t.Swap} {
		if !in(b.Warn) || !in(b.Critical) {
			return false
		}
	}
	return t.Disk.Critical <= t.Disk.Warn && t.Inodes.Critical <= t.Inodes.Warn &&
		t.Memory.Critical <= t.Memory.Warn && t.Swap.Critical >= t.Swap.Warn
}

// Sample is one reading of a node's host. A zero total means the value is unreadable.
type Sample struct {
	DiskTotal, DiskFree     uint64
	InodesTotal, InodesFree uint64
	MemTotal, MemAvail      uint64
	SwapTotal, SwapFree     uint64
}

// MetricStatus is one metric's reading and verdict.
type MetricStatus struct {
	Name    string  `json:"name"`
	Level   Level   `json:"level"`
	Percent float64 `json:"percent"`
	// Free and Total are bytes, or counts for inodes.
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
}

// Status is a node's overall verdict.
type Status struct {
	Metrics    []MetricStatus `json:"metrics"`
	Thresholds Thresholds     `json:"thresholds"`
	Worst      Level          `json:"worst"`
	// Reason names the worst metric in words, empty when everything is fine.
	Reason string `json:"reason,omitempty"`
}

func rank(l Level) int {
	switch l {
	case LevelWarn:
		return 1
	case LevelCritical:
		return 2
	}
	return 0
}

func pct(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}

func freeLevel(freePct float64, b Band) Level {
	switch {
	case freePct <= b.Critical:
		return LevelCritical
	case freePct <= b.Warn:
		return LevelWarn
	}
	return LevelOK
}

// Evaluate judges a sample against thresholds. Unreadable metrics are reported as unknown and never block.
func Evaluate(s Sample, t Thresholds) Status {
	st := Status{Thresholds: t, Worst: LevelOK, Metrics: []MetricStatus{}}
	add := func(name string, total, free uint64, band Band, used bool) {
		m := MetricStatus{Name: name, Level: LevelUnknown, Free: free, Total: total}
		if total > 0 {
			free = min(free, total)
			if used {
				m.Percent = pct(total-free, total)
				switch {
				case m.Percent >= band.Critical:
					m.Level = LevelCritical
				case m.Percent >= band.Warn:
					m.Level = LevelWarn
				default:
					m.Level = LevelOK
				}
			} else {
				m.Percent = pct(free, total)
				m.Level = freeLevel(m.Percent, band)
			}
		}
		if rank(m.Level) > rank(st.Worst) {
			st.Worst = m.Level
			st.Reason = reasonFor(m, used)
		}
		st.Metrics = append(st.Metrics, m)
	}
	add(MetricDisk, s.DiskTotal, s.DiskFree, t.Disk, false)
	add(MetricInodes, s.InodesTotal, s.InodesFree, t.Inodes, false)
	add(MetricMemory, s.MemTotal, s.MemAvail, t.Memory, false)
	add(MetricSwap, s.SwapTotal, s.SwapFree, t.Swap, true)
	return st
}

func reasonFor(m MetricStatus, used bool) string {
	label := map[string]string{MetricDisk: "disk space", MetricInodes: "inodes", MetricMemory: "available memory", MetricSwap: "swap"}[m.Name]
	p := strconv.FormatFloat(m.Percent, 'f', 1, 64)
	if used {
		return label + " is " + p + "% used"
	}
	return label + " is down to " + p + "% free"
}

// Blocking reports whether new builds and deploys must be refused.
func (s Status) Blocking() bool { return s.Worst == LevelCritical }
