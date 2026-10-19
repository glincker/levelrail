// Package cphealth turns raw control plane measurements into saturation findings with typed fixes.
package cphealth

import (
	"os"
	"strconv"
	"time"
)

// Severity ranks a finding.
type Severity string

// Severity values.
const (
	SeverityWarn Severity = "warn"
	SeverityCrit Severity = "crit"
)

// Fix ids the API can apply. Empty means the finding needs the operator.
const (
	FixWALCheckpoint  = "wal_checkpoint"
	FixDBOptimize     = "db_optimize"
	FixResetSlowLog   = "reset_slow_log"
	FixTelemetryPrune = "telemetry_prune"
	FixResetGuard     = "reset_reconcile_guard"
	FixIntegrityCheck = "integrity_check"
	FixIntegrityFix   = "integrity_restore"
)

// Finding ids.
const (
	FindingGoroutines = "goroutines"
	FindingHeap       = "heap"
	FindingDBLatency  = "db_latency"
	FindingDBQueue    = "db_queue"
	FindingWAL        = "wal_size"
	FindingSlowQuery  = "slow_queries"
	FindingReconcile  = "reconcile_pass"
	FindingEventLag   = "event_lag"
	FindingStreams    = "streams"
	FindingRSS        = "rss"
	FindingThreads    = "threads"
	FindingTelemetry  = "telemetry_size"
	FindingBreakers   = "reconcile_breakers"
	FindingStorm      = "reconcile_storm"
	FindingIntegrity  = "db_integrity"
)

// Inputs are the measurements one evaluation reads.
type Inputs struct {
	Goroutines     int
	HeapBytes      uint64
	DBP95MS        float64
	DBWaitCount    int64
	DBInUse        int
	WALBytes       int64
	SlowQueries    int64
	ReconcilePass  time.Duration
	LastPassAt     time.Time
	ResyncInterval time.Duration
	RSSBytes       uint64
	Threads        int
	TelemetryBytes int64
	Streams        int
	StreamCap      int
	Now            time.Time

	OpenBreakers    int
	RecentLimited   int
	IntegrityFailed bool
}

// Thresholds are the limits above which a measurement is flagged. Env vars override the defaults.
type Thresholds struct {
	Goroutines     int           `json:"goroutines"`
	HeapBytes      uint64        `json:"heap_bytes"`
	DBP95MS        float64       `json:"db_p95_ms"`
	WALBytes       int64         `json:"wal_bytes"`
	SlowQueries    int64         `json:"slow_queries"`
	ReconcilePass  time.Duration `json:"reconcile_pass_ns"`
	EventLagMult   int           `json:"event_lag_multiple"`
	StreamPercent  int           `json:"stream_percent"`
	RSSBytes       uint64        `json:"rss_bytes"`
	Threads        int           `json:"threads"`
	TelemetryBytes int64         `json:"telemetry_bytes"`
	StormEvents    int           `json:"storm_events"`
}

// Finding is one saturated or degraded measurement.
type Finding struct {
	ID        string   `json:"id"`
	Severity  Severity `json:"severity"`
	Value     float64  `json:"value"`
	Threshold float64  `json:"threshold"`
	Fix       string   `json:"fix,omitempty"`
}

// DefaultThresholds reads the APP_CPHEALTH_* env vars over built in defaults.
func DefaultThresholds(getenv func(string) string) Thresholds {
	if getenv == nil {
		getenv = os.Getenv
	}
	num := func(key string, def float64) float64 {
		if v, err := strconv.ParseFloat(getenv(key), 64); err == nil && v > 0 {
			return v
		}
		return def
	}
	return Thresholds{
		Goroutines:     int(num("APP_CPHEALTH_GOROUTINES", 5000)),
		HeapBytes:      uint64(num("APP_CPHEALTH_HEAP_MB", 1536)) << 20,
		DBP95MS:        num("APP_CPHEALTH_DB_P95_MS", 100),
		WALBytes:       int64(num("APP_CPHEALTH_WAL_MB", 128)) << 20,
		SlowQueries:    int64(num("APP_CPHEALTH_SLOW_QUERIES", 25)),
		ReconcilePass:  time.Duration(num("APP_CPHEALTH_RECONCILE_PASS_S", 20) * float64(time.Second)),
		EventLagMult:   int(num("APP_CPHEALTH_EVENT_LAG_MULTIPLE", 3)),
		StreamPercent:  int(num("APP_CPHEALTH_STREAM_PERCENT", 80)),
		RSSBytes:       uint64(num("APP_CPHEALTH_RSS_MB", 1024)) << 20,
		Threads:        int(num("APP_CPHEALTH_THREADS", 200)),
		TelemetryBytes: int64(num("APP_CPHEALTH_TELEMETRY_MB", 768)) << 20,
		StormEvents:    int(num("APP_CPHEALTH_STORM_EVENTS", 100)),
	}
}

func rank(value, warn float64) Severity {
	if value >= warn*2 {
		return SeverityCrit
	}
	return SeverityWarn
}

// Evaluate returns the findings for in against t, worst first is not guaranteed; callers sort by severity.
func Evaluate(in Inputs, t Thresholds) []Finding {
	var out []Finding
	add := func(id string, v, limit float64, fix string) {
		out = append(out, Finding{ID: id, Severity: rank(v, limit), Value: v, Threshold: limit, Fix: fix})
	}
	if in.IntegrityFailed {
		out = append(out, Finding{ID: FindingIntegrity, Severity: SeverityCrit, Value: 1, Threshold: 0, Fix: FixIntegrityFix})
	}
	if in.OpenBreakers > 0 {
		add(FindingBreakers, float64(in.OpenBreakers), 1, FixResetGuard)
	}
	if t.StormEvents > 0 && in.RecentLimited > t.StormEvents {
		add(FindingStorm, float64(in.RecentLimited), float64(t.StormEvents), FixResetGuard)
	}
	if in.Goroutines > t.Goroutines {
		add(FindingGoroutines, float64(in.Goroutines), float64(t.Goroutines), "")
	}
	if in.HeapBytes > t.HeapBytes {
		add(FindingHeap, float64(in.HeapBytes), float64(t.HeapBytes), "")
	}
	if in.RSSBytes > t.RSSBytes {
		add(FindingRSS, float64(in.RSSBytes), float64(t.RSSBytes), "")
	}
	if in.Threads > t.Threads {
		add(FindingThreads, float64(in.Threads), float64(t.Threads), "")
	}
	if in.TelemetryBytes > t.TelemetryBytes {
		add(FindingTelemetry, float64(in.TelemetryBytes), float64(t.TelemetryBytes), FixTelemetryPrune)
	}
	if in.DBP95MS > t.DBP95MS {
		add(FindingDBLatency, in.DBP95MS, t.DBP95MS, FixDBOptimize)
	}
	if in.DBInUse > 0 && in.DBWaitCount > 0 && in.DBP95MS > t.DBP95MS/2 {
		add(FindingDBQueue, float64(in.DBWaitCount), 1, "")
	}
	if in.WALBytes > t.WALBytes {
		add(FindingWAL, float64(in.WALBytes), float64(t.WALBytes), FixWALCheckpoint)
	}
	if in.SlowQueries > t.SlowQueries {
		add(FindingSlowQuery, float64(in.SlowQueries), float64(t.SlowQueries), FixResetSlowLog)
	}
	if in.ReconcilePass > t.ReconcilePass {
		add(FindingReconcile, in.ReconcilePass.Seconds(), t.ReconcilePass.Seconds(), "")
	}
	if in.ResyncInterval > 0 && !in.LastPassAt.IsZero() && t.EventLagMult > 0 {
		lag := in.Now.Sub(in.LastPassAt)
		if limit := in.ResyncInterval * time.Duration(t.EventLagMult); lag > limit {
			add(FindingEventLag, lag.Seconds(), limit.Seconds(), "")
		}
	}
	if in.StreamCap > 0 && in.Streams*100 >= in.StreamCap*t.StreamPercent {
		add(FindingStreams, float64(in.Streams), float64(in.StreamCap), "")
	}
	return out
}
