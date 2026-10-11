// Package usagecost allocates a real or configured infrastructure price to
// the apps and databases that consume it, then rolls the result up per
// project, environment and team. It is pure arithmetic: collection and
// persistence live with the callers.
package usagecost

import "time"

// Allocation modes.
const (
	ModeUnit = "unit"
	ModeNode = "node"
)

// Usage bases.
const (
	BasisAvg = "avg"
	BasisP50 = "p50"
)

// Row kinds a daily row can describe.
const (
	KindApp      = "app"
	KindDatabase = "database"
	KindIdle     = "idle"
	KindDay      = "day"
)

const (
	secondsPerDay = 86400.0
	hoursPerDay   = 24.0
	gib           = 1 << 30
)

// Settings is the operator's pricing configuration.
type Settings struct {
	Mode            string  `json:"mode"`
	Currency        string  `json:"currency"`
	CPUCoreHour     float64 `json:"cpu_core_hour"`
	MemGiBHour      float64 `json:"mem_gib_hour"`
	StorageGiBMonth float64 `json:"storage_gib_month"`
	EgressGiB       float64 `json:"egress_gib"`
	// IdleSpreadPct is, in node mode, the share of unattributed node cost
	// spread over resources by usage; in unit mode a proportional overhead.
	IdleSpreadPct float64 `json:"idle_spread_pct"`
	Basis         string  `json:"basis"`
	// CPUWeightPct is the part of a node price attributed to CPU, the rest
	// to memory.
	CPUWeightPct float64 `json:"cpu_weight_pct"`
}

// DefaultSettings is what an unconfigured instance reports: unit mode with
// zero prices, so nothing is invented until the operator sets prices.
func DefaultSettings() Settings {
	return Settings{Mode: ModeUnit, Currency: "USD", Basis: BasisAvg, CPUWeightPct: 50}
}

// NodePrice is one node's monthly bill and capacity.
type NodePrice struct {
	NodeID       string  `json:"node_id"`
	MonthlyPrice float64 `json:"monthly_price"`
	VCPUs        float64 `json:"vcpus"`
	MemoryGiB    float64 `json:"memory_gib"`
}

// Priced reports whether the node can be used for allocation.
func (n NodePrice) Priced() bool {
	return n.MonthlyPrice > 0 && (n.VCPUs > 0 || n.MemoryGiB > 0)
}

// DailyUsage is one resource's measured usage for one UTC day.
type DailyUsage struct {
	Day           string  `json:"day"`
	Kind          string  `json:"kind"`
	Name          string  `json:"name"`
	TeamID        string  `json:"team_id"`
	ProjectID     string  `json:"project_id"`
	EnvironmentID string  `json:"environment_id"`
	NodeID        string  `json:"node_id"`
	UpSeconds     float64 `json:"up_seconds"`
	CPUAvgCores   float64 `json:"cpu_avg_cores"`
	CPUP50Cores   float64 `json:"cpu_p50_cores"`
	MemAvgBytes   float64 `json:"mem_avg_bytes"`
	MemP50Bytes   float64 `json:"mem_p50_bytes"`
	StorageBytes  float64 `json:"storage_bytes"`
	EgressBytes   float64 `json:"egress_bytes"`
}

// DailyCost is a DailyUsage with its allocated cost components.
type DailyCost struct {
	DailyUsage
	ComputeCost float64 `json:"compute_cost"`
	StorageCost float64 `json:"storage_cost"`
	EgressCost  float64 `json:"egress_cost"`
	IdleCost    float64 `json:"idle_cost"`
}

// Total is the full cost attributed to the row.
func (c DailyCost) Total() float64 {
	return c.ComputeCost + c.StorageCost + c.EgressCost + c.IdleCost
}

// DayResult is the allocation of one day.
type DayResult struct {
	Rows []DailyCost
	// Unpriced lists node ids whose resources could not be priced.
	Unpriced []string
}

// Period is a half open [From, To) range of UTC days.
type Period struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Days returns the day count of the period.
func (p Period) Days() int {
	return int(p.To.Sub(p.From).Hours() / hoursPerDay)
}

// DayKey formats a time as the UTC day key stored in snapshots.
func DayKey(t time.Time) string { return t.UTC().Format("2006-01-02") }

// ParseDay parses a day key.
func ParseDay(s string) (time.Time, error) { return time.Parse("2006-01-02", s) }

// StartOfDay truncates to 00:00 UTC.
func StartOfDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// StartOfMonth truncates to the first day of the UTC month.
func StartOfMonth(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// DaysInMonth returns how many days the month containing t has.
func DaysInMonth(t time.Time) int {
	return StartOfMonth(t).AddDate(0, 1, 0).Add(-time.Second).Day()
}
