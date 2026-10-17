package placement

import (
	"fmt"
	"strings"
)

// Amount is a measured quantity. Known is false when the node never
// reported it: unknown is not zero (nothing fits) and not infinite
// (everything fits), it is "cannot be verified".
type Amount struct {
	Value int64 `json:"value"`
	Known bool  `json:"known"`
}

// Measured returns a known Amount.
func Measured(v int64) Amount { return Amount{Value: v, Known: true} }

// Resources is what a workload asks for, or what placed workloads add up to.
type Resources struct {
	CPUMilli    int64 `json:"cpu_milli"`
	MemoryBytes int64 `json:"memory_bytes"`
	DiskBytes   int64 `json:"disk_bytes"`
}

// Add returns r plus o.
func (r Resources) Add(o Resources) Resources {
	return Resources{r.CPUMilli + o.CPUMilli, r.MemoryBytes + o.MemoryBytes, r.DiskBytes + o.DiskBytes}
}

// Times returns r multiplied by n, the replica count.
func (r Resources) Times(n int64) Resources {
	return Resources{r.CPUMilli * n, r.MemoryBytes * n, r.DiskBytes * n}
}

// Capacity is what a node reports about itself.
type Capacity struct {
	CPUMilli       Amount `json:"cpu_milli"`
	MemoryBytes    Amount `json:"memory_bytes"`
	DiskTotalBytes Amount `json:"disk_total_bytes"`
	DiskFreeBytes  Amount `json:"disk_free_bytes"`
}

// Policy is the operator's overcommit and headroom setting.
// Percent 100 means requests may add up to the node's measured total and
// no further; above 100 allows overcommit. MinFreeDiskPercent keeps that
// share of the disk unplaceable. RejectUnknown filters nodes whose
// capacity cannot be verified instead of ranking them last.
type Policy struct {
	CPUOvercommitPercent    int  `json:"cpu_overcommit_percent"`
	MemoryOvercommitPercent int  `json:"memory_overcommit_percent"`
	MinFreeDiskPercent      int  `json:"min_free_disk_percent"`
	RejectUnknown           bool `json:"reject_unknown"`
}

// DefaultPolicy never overcommits, keeps 10 percent of disk free and ranks
// unverifiable nodes last rather than refusing them.
func DefaultPolicy() Policy {
	return Policy{CPUOvercommitPercent: 100, MemoryOvercommitPercent: 100, MinFreeDiskPercent: 10}
}

// Validate rejects percentages outside their useful range.
func (p Policy) Validate() error {
	if p.CPUOvercommitPercent < 1 || p.CPUOvercommitPercent > 1000 {
		return fmt.Errorf("cpu overcommit percent must be 1 to 1000, got %d", p.CPUOvercommitPercent)
	}
	if p.MemoryOvercommitPercent < 1 || p.MemoryOvercommitPercent > 1000 {
		return fmt.Errorf("memory overcommit percent must be 1 to 1000, got %d", p.MemoryOvercommitPercent)
	}
	if p.MinFreeDiskPercent < 0 || p.MinFreeDiskPercent > 90 {
		return fmt.Errorf("minimum free disk percent must be 0 to 90, got %d", p.MinFreeDiskPercent)
	}
	return nil
}

func (p Policy) withDefaults() Policy {
	if p.CPUOvercommitPercent == 0 {
		p.CPUOvercommitPercent = 100
	}
	if p.MemoryOvercommitPercent == 0 {
		p.MemoryOvercommitPercent = 100
	}
	return p
}

// Dimension names in Evaluation.
const (
	DimCPU    = "cpu"
	DimMemory = "memory"
	DimDisk   = "disk"
)

// Dimension is one resource on one node, in the unit the operator reads.
// Allocatable is nil when the node never reported it. For disk, Requested
// is measured use and Allocatable already excludes the free-space floor.
type Dimension struct {
	Name        string `json:"name"`
	Allocatable *int64 `json:"allocatable"`
	Requested   int64  `json:"requested"`
	Need        int64  `json:"need"`
	Free        *int64 `json:"free"`
	Shortfall   int64  `json:"shortfall"`
}

func dimension(name string, capacity Amount, pct int, requested, need int64) Dimension {
	d := Dimension{Name: name, Requested: requested, Need: need}
	if !capacity.Known {
		return d
	}
	alloc := capacity.Value * int64(pct) / 100
	free := alloc - requested
	d.Allocatable, d.Free = &alloc, &free
	if need > free {
		d.Shortfall = need - free
	}
	return d
}

func diskDimension(c Capacity, floorPct int, need int64) Dimension {
	d := Dimension{Name: DimDisk, Need: need}
	if !c.DiskTotalBytes.Known || !c.DiskFreeBytes.Known {
		return d
	}
	total := c.DiskTotalBytes.Value
	floor := total * int64(floorPct) / 100
	used := max(total-c.DiskFreeBytes.Value, 0)
	alloc := max(total-floor, 0)
	free := alloc - used
	d.Allocatable, d.Requested, d.Free = &alloc, used, &free
	if need > free {
		d.Shortfall = need - free
	}
	return d
}

// Describe renders d's shortfall or state for an operator.
func (d Dimension) Describe() string {
	if d.Allocatable == nil {
		return d.Name + ": capacity not reported"
	}
	free := max(*d.Free, 0)
	if d.Shortfall > 0 {
		return fmt.Sprintf("%s: needs %s, %s free of %s allocatable (short by %s)",
			d.Name, d.format(d.Need), d.format(free), d.format(*d.Allocatable), d.format(d.Shortfall))
	}
	return fmt.Sprintf("%s: %s free of %s allocatable", d.Name, d.format(free), d.format(*d.Allocatable))
}

func (d Dimension) format(v int64) string {
	if d.Name == DimCPU {
		return FormatCPU(v)
	}
	return FormatBytes(v)
}

// FormatCPU renders millicores as cores.
func FormatCPU(milli int64) string {
	s := fmt.Sprintf("%.2f", float64(milli)/1000)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		s = "0"
	}
	return s + " CPU"
}

// FormatBytes renders bytes with a binary unit.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTP"[exp])
}
