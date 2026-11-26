// Package dbadvisor reads Postgres statistics through the viewer's read-only exec path and turns them into
// findings with evidence and a suggested fix as text. Nothing it suggests is ever executed.
package dbadvisor

import (
	"errors"
	"time"
)

// ErrUnsupported reports an engine the advisor has no catalog queries for.
var ErrUnsupported = errors.New("performance advisor supports Postgres only")

// Severities, ordered from least to most urgent.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)

// Finding kinds.
const (
	KindUnusedIndex   = "unused_index"
	KindMissingFKIdx  = "missing_fk_index"
	KindSeqScan       = "high_seq_scan"
	KindBloat         = "dead_tuples"
	KindCacheHit      = "low_cache_hit"
	KindSlowStatement = "slow_statement"
	KindStatementsOff = "statements_not_enabled"
	KindLongSession   = "long_session"
	KindCheckFailed   = "check_failed"
)

// Finding is one observation. Suggestion is text for a human to review and try on a branch first.
type Finding struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Severity   string         `json:"severity"`
	Title      string         `json:"title"`
	Object     string         `json:"object,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	Suggestion string         `json:"suggestion,omitempty"`
	Docs       string         `json:"docs,omitempty"`
}

// Report is the whole advisor answer for one database.
type Report struct {
	Engine    string    `json:"engine"`
	CheckedAt time.Time `json:"checked_at"`
	Findings  []Finding `json:"findings"`
	// Truncated is true when more findings existed than the configured cap.
	Truncated bool `json:"truncated"`
}

// Thresholds are the live settings one run uses.
type Thresholds struct {
	UnusedIndexMinBytes int64
	SeqScanMinRows      int64
	// SeqScanRatioPercent flags tables where sequential scans are at least this share of all scans.
	SeqScanRatioPercent int
	DeadTuplePercent    int
	CacheHitMinPercent  int
	IdleInTxMinutes     int
	SlowStatementMs     int
	MaxFindings         int
	Timeout             time.Duration
}
