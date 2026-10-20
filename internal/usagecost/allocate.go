package usagecost

import (
	"math"
	"sort"
	"time"
)

// DayInput is everything allocation needs for one UTC day.
type DayInput struct {
	Day      time.Time
	Settings Settings
	Nodes    map[string]NodePrice
	Usage    []DailyUsage
	// Fraction is the share of the day that has elapsed, 1 for a complete
	// day. A partial day scales the node's daily price the same way.
	Fraction float64
}

func (s Settings) normalized() Settings {
	d := DefaultSettings()
	if s.Mode == "" {
		s.Mode = d.Mode
	}
	if s.Basis == "" {
		s.Basis = d.Basis
	}
	if s.CPUWeightPct <= 0 || s.CPUWeightPct > 100 {
		s.CPUWeightPct = d.CPUWeightPct
	}
	s.IdleSpreadPct = math.Min(100, math.Max(0, s.IdleSpreadPct))
	return s
}

func (u DailyUsage) cores(basis string) float64 {
	if basis == BasisP50 {
		return u.CPUP50Cores
	}
	return u.CPUAvgCores
}

func (u DailyUsage) memBytes(basis string) float64 {
	if basis == BasisP50 {
		return u.MemP50Bytes
	}
	return u.MemAvgBytes
}

// AllocateDay prices one day of usage. Storage and egress unit prices apply
// in both modes; compute is priced per unit or taken from node bills.
func AllocateDay(in DayInput) DayResult {
	s := in.Settings.normalized()
	frac := in.Fraction
	if frac <= 0 || frac > 1 {
		frac = 1
	}
	days := float64(DaysInMonth(in.Day))
	rows := make([]DailyCost, 0, len(in.Usage))
	for _, u := range in.Usage {
		rows = append(rows, DailyCost{
			DailyUsage:  u,
			StorageCost: u.StorageBytes / gib * s.StorageGiBMonth / days * frac,
			EgressCost:  u.EgressBytes / gib * s.EgressGiB,
		})
	}
	var res DayResult
	var idleRows []DailyCost
	if s.Mode == ModeNode {
		res.Unpriced, idleRows = allocateNodes(rows, in.Nodes, s, days, frac, in.Day)
	} else {
		allocateUnits(rows, s)
	}
	sortRows(rows)
	res.Rows = append(rows, idleRows...)
	return res
}

func allocateUnits(rows []DailyCost, s Settings) {
	for i := range rows {
		u := rows[i].DailyUsage
		hours := u.UpSeconds / 3600
		compute := u.cores(s.Basis)*hours*s.CPUCoreHour + u.memBytes(s.Basis)/gib*hours*s.MemGiBHour
		rows[i].ComputeCost = compute
		rows[i].IdleCost = compute * s.IdleSpreadPct / 100
	}
}

// allocateNodes splits each priced node's daily bill by usage share, scales
// down on overcommit, spreads IdleSpreadPct of the remainder proportionally
// and emits the rest as an idle row. Resources on unpriced nodes cost zero
// compute and their node ids are returned.
func allocateNodes(rows []DailyCost, nodes map[string]NodePrice, s Settings, days, frac float64, day time.Time) (unpriced []string, idleRows []DailyCost) {
	byNode := map[string][]int{}
	for i := range rows {
		byNode[rows[i].NodeID] = append(byNode[rows[i].NodeID], i)
	}
	ids := make([]string, 0, len(nodes)+len(byNode))
	for id := range nodes {
		ids = append(ids, id)
	}
	for id := range byNode {
		if _, ok := nodes[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		np, ok := nodes[id]
		if !ok || !np.Priced() {
			if len(byNode[id]) > 0 {
				unpriced = append(unpriced, id)
			}
			continue
		}
		daily := np.MonthlyPrice / days * frac
		idle := allocateOneNode(rows, byNode[id], np, daily, s)
		if idle > 1e-9 {
			idleRows = append(idleRows, DailyCost{
				DailyUsage: DailyUsage{Day: DayKey(day), Kind: KindIdle, Name: id, NodeID: id},
				IdleCost:   idle,
			})
		}
	}
	return unpriced, idleRows
}

func allocateOneNode(rows []DailyCost, idx []int, np NodePrice, daily float64, s Settings) float64 {
	cpuW, memW := s.CPUWeightPct/100, 1-s.CPUWeightPct/100
	if np.VCPUs <= 0 {
		cpuW, memW = 0, 1
	}
	if np.MemoryGiB <= 0 {
		cpuW, memW = 1, 0
	}
	used := make([]float64, len(idx))
	sum := 0.0
	for k, i := range idx {
		u := rows[i].DailyUsage
		share := 0.0
		if np.VCPUs > 0 {
			share += cpuW * u.cores(s.Basis) * u.UpSeconds / secondsPerDay / np.VCPUs
		}
		if np.MemoryGiB > 0 {
			share += memW * u.memBytes(s.Basis) / gib * u.UpSeconds / secondsPerDay / np.MemoryGiB
		}
		used[k] = daily * share
		sum += used[k]
	}
	if sum > daily {
		scale := daily / sum
		for k := range used {
			used[k] *= scale
		}
		sum = daily
	}
	idle := daily - sum
	spread := 0.0
	if sum > 0 {
		spread = idle * s.IdleSpreadPct / 100
	}
	for k, i := range idx {
		rows[i].ComputeCost = used[k]
		if sum > 0 {
			rows[i].IdleCost = spread * used[k] / sum
		}
	}
	return idle - spread
}

func sortRows(rows []DailyCost) {
	sort.SliceStable(rows, func(a, b int) bool {
		if rows[a].Kind != rows[b].Kind {
			return rows[a].Kind < rows[b].Kind
		}
		return rows[a].Name < rows[b].Name
	})
}

// hoursPerMonth is the conventional billing month used to turn a monthly
// price into an hourly one.
const hoursPerMonth = 730.0

// UnitPricesFromNode derives per-core-hour and per-GiB-hour prices from one
// node's monthly price and capacity, splitting the price by cpuWeightPct.
func UnitPricesFromNode(n NodePrice, cpuWeightPct float64) (cpuCoreHour, memGiBHour float64) {
	if cpuWeightPct <= 0 || cpuWeightPct > 100 {
		cpuWeightPct = DefaultSettings().CPUWeightPct
	}
	w := cpuWeightPct / 100
	if n.VCPUs > 0 {
		cpuCoreHour = n.MonthlyPrice * w / (n.VCPUs * hoursPerMonth)
	}
	if n.MemoryGiB > 0 {
		memGiBHour = n.MonthlyPrice * (1 - w) / (n.MemoryGiB * hoursPerMonth)
	}
	return cpuCoreHour, memGiBHour
}
