// Package resourceadvisor turns telemetry, limits and specs into read-only
// resource findings. It is pure: no I/O, no clock, never applies anything.
package resourceadvisor

import (
	"fmt"
	"strings"
	"time"
)

// Severity ranks a finding.
type Severity string

// Severities, most urgent last.
const (
	SeverityInfo    Severity = "info"
	SeveritySuggest Severity = "suggest"
	SeverityWarn    Severity = "warn"
)

// Rank orders severities, warn highest.
func (s Severity) Rank() int {
	switch s {
	case SeverityWarn:
		return 2
	case SeveritySuggest:
		return 1
	}
	return 0
}

// Kind names a rule.
type Kind string

// Finding kinds, one per rule.
const (
	KindMemoryLimitMissing    Kind = "memory_limit_missing"
	KindCPULimitMissing       Kind = "cpu_limit_missing"
	KindMemoryLimitTight      Kind = "memory_limit_tight"
	KindCPULimitTight         Kind = "cpu_limit_tight"
	KindMemoryOverProvisioned Kind = "memory_over_provisioned"
	KindCPUOverProvisioned    Kind = "cpu_over_provisioned"
	KindMemoryGrowth          Kind = "memory_growth"
	KindIdleApp               Kind = "idle_app"
	KindIdleDatabase          Kind = "idle_database"
	KindEngineTuning          Kind = "engine_tuning"
	KindConsolidate           Kind = "consolidate_databases"
	KindDiskForecast          Kind = "disk_forecast"
	KindReclaimableDisk       Kind = "reclaimable_disk"
	KindHostMemoryPressure    Kind = "host_memory_pressure"
	KindSwapInUse             Kind = "swap_in_use"
	KindTopConsumers          Kind = "top_consumers"
	KindCapacityUnknown       Kind = "capacity_unknown"
	KindNotEnoughData         Kind = "not_enough_data"
)

// ResourceKind is what a finding is about.
type ResourceKind string

// Resource kinds.
const (
	ResourceApp      ResourceKind = "app"
	ResourceDatabase ResourceKind = "database"
	ResourceNode     ResourceKind = "node"
)

// Ref names one resource.
type Ref struct {
	Kind ResourceKind `json:"kind"`
	Name string       `json:"name"`
}

func (r Ref) String() string { return string(r.Kind) + ":" + r.Name }

// ParseRef parses "app:web", "database:main" or "node:id".
func ParseRef(s string) (Ref, error) {
	kind, name, ok := strings.Cut(s, ":")
	if !ok || name == "" {
		return Ref{}, fmt.Errorf("resource %q: want kind:name", s)
	}
	switch k := ResourceKind(kind); k {
	case ResourceApp, ResourceDatabase, ResourceNode:
		return Ref{Kind: k, Name: name}, nil
	}
	return Ref{}, fmt.Errorf("resource %q: kind must be app, database or node", s)
}

// ActionType is what applying a finding would do.
type ActionType string

// Action types. Only the first three can be applied through the advisor.
const (
	ActionSetMemoryLimit ActionType = "set_memory_limit"
	ActionSetCPULimit    ActionType = "set_cpu_limit"
	ActionEnableSleep    ActionType = "enable_sleep"
	ActionPruneDocker    ActionType = "prune_docker"
	ActionManual         ActionType = "manual"
)

// Action is a concrete next step.
type Action struct {
	Type ActionType `json:"type"`
	// Value is bytes, nano-CPUs or idle minutes, by Type. Current is the
	// value it replaces, 0 when none is set.
	Value        int64             `json:"value,omitempty"`
	Current      int64             `json:"current,omitempty"`
	Settings     map[string]string `json:"settings,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
}

// Appliable reports whether the advisor can apply this action itself.
func (a Action) Appliable() bool {
	switch a.Type {
	case ActionSetMemoryLimit, ActionSetCPULimit, ActionEnableSleep:
		return a.Value > 0
	}
	return false
}

// Point is one downsampled evidence reading.
type Point struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// Evidence is the data behind a finding.
type Evidence struct {
	Window        string             `json:"window"`
	WindowSeconds int64              `json:"window_seconds"`
	Samples       int                `json:"samples"`
	Unit          string             `json:"unit,omitempty"`
	P50           float64            `json:"p50,omitempty"`
	P95           float64            `json:"p95,omitempty"`
	P99           float64            `json:"p99,omitempty"`
	Max           float64            `json:"max,omitempty"`
	Limit         float64            `json:"limit,omitempty"`
	Numbers       map[string]float64 `json:"numbers,omitempty"`
	Series        []Point            `json:"series,omitempty"`
}

// Evidence units.
const (
	UnitBytes    = "bytes"
	UnitNanoCPUs = "nano_cpus"
	UnitRequests = "requests"
)

// Impact is the estimated saving or risk.
type Impact struct {
	SavingMemoryBytes int64      `json:"saving_memory_bytes,omitempty"`
	SavingNanoCPUs    int64      `json:"saving_nano_cpus,omitempty"`
	SavingDiskBytes   int64      `json:"saving_disk_bytes,omitempty"`
	ExhaustionAt      *time.Time `json:"exhaustion_at,omitempty"`
	Risk              string     `json:"risk,omitempty"`
}

// Finding is one piece of advice.
type Finding struct {
	ID       string            `json:"id"`
	Kind     Kind              `json:"kind"`
	Severity Severity          `json:"severity"`
	Resource Ref               `json:"resource"`
	Title    string            `json:"title"`
	Why      string            `json:"why"`
	Evidence Evidence          `json:"evidence"`
	Impact   Impact            `json:"impact"`
	Action   *Action           `json:"action,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

func newFinding(kind Kind, sev Severity, ref Ref) Finding {
	return Finding{ID: string(kind) + ":" + ref.String(), Kind: kind, Severity: sev, Resource: ref, Params: map[string]string{"name": ref.Name}}
}

// Sample is one telemetry reading.
type Sample struct {
	At    time.Time
	Value float64
}

// OOM is an observed OOM-kill signal.
type OOM struct {
	At      time.Time
	Excerpt string
}

// Resource is one app or database and everything known about it.
type Resource struct {
	Ref     Ref
	NodeID  string
	Engine  string
	Version string
	Image   string
	// MemoryLimit and NanoCPUs are the configured limits, 0 when unset.
	MemoryLimit int64
	NanoCPUs    int64
	Memory      []Sample
	CPUPercent  []Sample
	Restarts    int
	OOM         *OOM
	// Requests are per-tick request counts; only non-zero ticks are stored,
	// so they mean anything only when ServesTraffic is true.
	Requests      []Sample
	ServesTraffic bool
	SleepEnabled  bool
	Suspended     bool
	// LinkedApps counts apps wired to a database, -1 when unknown.
	LinkedApps int
}

// Reclaimable is Docker's own reclaimable disk accounting.
type Reclaimable struct {
	ImagesBytes     int64
	BuildCacheBytes int64
}

// Node is one machine.
type Node struct {
	ID   string
	Name string
	// MemoryTotal is 0 when the node does not report host memory.
	MemoryTotal     float64
	MemoryAvailable []Sample
	SwapUsed        []Sample
	DiskUsed        []Sample
	DiskTotal       float64
	Reclaimable     *Reclaimable
	// NoIdentity marks a single server without a node record.
	NoIdentity bool
}

// Input is everything Analyze reads.
type Input struct {
	Now        time.Time
	Resources  []Resource
	Nodes      []Node
	Thresholds Thresholds
}
