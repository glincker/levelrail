package analysis

import "time"

// SchemaVersion is stored with every report so readers can evolve.
const SchemaVersion = 1

// Report statuses.
const (
	StatusComplete     = "complete"
	StatusInsufficient = "insufficient_data"
)

// Trigger kinds.
const (
	TriggerAlert = "alert"
	TriggerSpend = "spend"
)

// Trigger is the alert or spend event a report answers.
type Trigger struct {
	Kind     string    `json:"kind"`
	ID       string    `json:"id"`
	App      string    `json:"app"`
	RuleName string    `json:"rule_name,omitempty"`
	Severity string    `json:"severity,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	At       time.Time `json:"at"`
}

// MetricStat summarises one series. Every number is computed in code.
type MetricStat struct {
	Evidence     string     `json:"evidence"`
	Metric       string     `json:"metric"`
	Samples      int        `json:"samples"`
	Min          float64    `json:"min"`
	P50          float64    `json:"p50"`
	P95          float64    `json:"p95"`
	Max          float64    `json:"max"`
	Mean         float64    `json:"mean"`
	BaselineMean float64    `json:"baseline_mean"`
	TrendPct     float64    `json:"trend_pct"`
	Trend        string     `json:"trend"`
	AnomalyCount int        `json:"anomaly_count"`
	MaxZ         float64    `json:"max_z"`
	FirstAnomaly *time.Time `json:"first_anomaly,omitempty"`
}

// LogGroup is one level plus normalized message fingerprint.
type LogGroup struct {
	Evidence    string    `json:"evidence"`
	Level       string    `json:"level"`
	Fingerprint string    `json:"fingerprint"`
	Sample      string    `json:"sample"`
	Count       int       `json:"count"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// Event is one timeline entry: a deploy, restart or alert firing.
type Event struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Ref    string    `json:"ref"`
	Detail string    `json:"detail,omitempty"`
}

// Sizing is the right-sizing view of one dimension, from internal/rightsizing.
type Sizing struct {
	Dimension      string  `json:"dimension"`
	Action         string  `json:"action"`
	Confidence     string  `json:"confidence"`
	DataSufficient bool    `json:"data_sufficient"`
	CurrentLimit   int64   `json:"current_limit"`
	P95Usage       float64 `json:"p95_usage"`
	SuggestedLimit int64   `json:"suggested_limit"`
	SuggestedText  string  `json:"suggested_text,omitempty"`
}

// Digest is the deterministic evidence bundle.
type Digest struct {
	App         string       `json:"app"`
	WindowStart time.Time    `json:"window_start"`
	WindowEnd   time.Time    `json:"window_end"`
	Metrics     []MetricStat `json:"metrics"`
	Logs        []LogGroup   `json:"logs"`
	LogLines    int          `json:"log_lines"`
	Restarts    int          `json:"restarts"`
	Timeline    []Event      `json:"timeline"`
	Sizing      []Sizing     `json:"sizing,omitempty"`
	// MonthlySavingUSD and SavingProvider come from internal/costestimate.
	MonthlySavingUSD float64 `json:"monthly_saving_usd,omitempty"`
	SavingProvider   string  `json:"saving_provider,omitempty"`
	// MemoryLimit and CPUPercentLimit are the declared limits (0 when unset).
	MemoryLimit     int64   `json:"memory_limit_bytes"`
	CPUPercentLimit float64 `json:"cpu_percent_limit"`
}

// Cause is one ranked suspected cause with the evidence ids that back it.
type Cause struct {
	Rank       int      `json:"rank"`
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Score      int      `json:"score"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

// Recommendation is a suggested operator action; applied by people only.
type Recommendation struct {
	ID               string   `json:"id"`
	Text             string   `json:"text"`
	MonthlySavingUSD float64  `json:"monthly_saving_usd,omitempty"`
	Evidence         []string `json:"evidence"`
}

// Report is the stored artifact.
type Report struct {
	SchemaVersion   int              `json:"schema_version"`
	Status          string           `json:"status"`
	App             string           `json:"app"`
	GeneratedAt     time.Time        `json:"generated_at"`
	Trigger         Trigger          `json:"trigger"`
	Summary         string           `json:"summary"`
	Timeline        []Event          `json:"timeline"`
	Causes          []Cause          `json:"causes"`
	Recommendations []Recommendation `json:"recommendations"`
	Digest          Digest           `json:"digest"`
}
