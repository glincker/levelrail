package runtimesettings

import (
	"context"
	"time"
)

// Registered keys for the database console and log rule limits. Env names are the long-standing ones and only seed defaults.
const (
	KeyDBViewerTimeout    = "database_viewer.query_timeout"
	KeyDBViewerMaxRows    = "database_viewer.max_rows"
	KeyDBViewerMaxBytes   = "database_viewer.max_bytes"
	KeyDBViewerMaxCell    = "database_viewer.max_cell_bytes"
	KeyDBViewerSchemaRows = "database_viewer.schema_max_rows"
	KeyDBViewerMaxSQL     = "database_viewer.max_sql_bytes"

	KeyLogRuleEvalInterval = "log_rules.eval_interval"
	KeyLogRuleMaxWindow    = "log_rules.max_window"
	KeyLogRuleMaxRows      = "log_rules.max_rows"
	KeyLogRuleSamples      = "log_rules.samples"
)

const (
	dbViewerMaxCount = 10_000_000
	logRuleMaxCount  = 1_000_000
)

// DBViewerLimits is the console's per-query bounds, read fresh on every request.
type DBViewerLimits struct {
	Timeout      time.Duration
	MaxRows      int
	MaxBytes     int
	MaxCellBytes int
	SchemaRows   int
	SQLBytes     int
}

// LogRuleLimits bounds one log rule evaluation.
type LogRuleLimits struct {
	EvalEvery time.Duration
	MaxWindow time.Duration
	MaxRows   int
	Samples   int
}

func registerLimitInt(key, env, def, title, desc string, floor, ceil int) {
	Register(Definition{
		Key: key, Category: CategoryObservability, Kind: KindInt, Default: def, Env: env, Apply: ApplyLive,
		CheckValue: intRangeCheck(floor, ceil), Title: title, Description: desc,
	})
}

func init() {
	cat := CategoryObservability
	Register(Definition{
		Key: KeyDBViewerTimeout, Category: cat, Kind: KindDuration, Default: "15s", Env: "APP_DB_QUERY_TIMEOUT", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Second, 10*time.Minute),
		Title:      "Database console: query timeout", Description: "How long one console query may run before it is cancelled, from 1s to 10m.",
	})
	registerLimitInt(KeyDBViewerMaxRows, "APP_DB_QUERY_MAX_ROWS", "1000", "Database console: max rows", "Rows one console query may return, from 1 to 10000000.", 1, dbViewerMaxCount)
	registerLimitInt(KeyDBViewerMaxBytes, "APP_DB_QUERY_MAX_BYTES", "4194304", "Database console: max result bytes", "Total result size one console query may return.", 1024, 1<<30)
	registerLimitInt(KeyDBViewerMaxCell, "APP_DB_QUERY_MAX_CELL_BYTES", "65536", "Database console: max cell bytes", "Largest single cell shown before it is cut off.", 64, 1<<24)
	registerLimitInt(KeyDBViewerSchemaRows, "APP_DB_SCHEMA_MAX_ROWS", "5000", "Database console: schema table limit", "Tables listed by the schema browser.", 1, dbViewerMaxCount)
	registerLimitInt(KeyDBViewerMaxSQL, "APP_DB_QUERY_MAX_SQL_BYTES", "65536", "Database console: max SQL bytes", "Largest SQL text the console and saved queries accept.", 256, 1<<24)

	Register(Definition{
		Key: KeyLogRuleEvalInterval, Category: cat, Kind: KindDuration, Default: "1m", Env: "APP_LOG_RULE_EVAL_INTERVAL", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Second, 24*time.Hour),
		Title:      "Log rules: evaluation interval", Description: "How often each log rule runs, from 1s to 24h.",
	})
	Register(Definition{
		Key: KeyLogRuleMaxWindow, Category: cat, Kind: KindDuration, Default: "1h", Env: "APP_LOG_RULE_MAX_WINDOW", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Minute, 720*time.Hour),
		Title:      "Log rules: longest look-back", Description: "Longest window a log rule may look back over, from 1m to 720h.",
	})
	registerLimitInt(KeyLogRuleMaxRows, "APP_LOG_RULE_MAX_ROWS", "5000", "Log rules: max rows per evaluation", "Log rows one rule evaluation may read.", 1, logRuleMaxCount)
	registerLimitInt(KeyLogRuleSamples, "APP_LOG_RULE_SAMPLES", "5", "Log rules: sample lines", "Matching lines attached to a firing alert.", 0, 100)
}

// DatabaseViewerLimits returns the live console limits; a nil service serves env then defaults.
func (s *Service) DatabaseViewerLimits(ctx context.Context) DBViewerLimits {
	return DBViewerLimits{
		Timeout:      s.Duration(ctx, KeyDBViewerTimeout),
		MaxRows:      s.Int(ctx, KeyDBViewerMaxRows),
		MaxBytes:     s.Int(ctx, KeyDBViewerMaxBytes),
		MaxCellBytes: s.Int(ctx, KeyDBViewerMaxCell),
		SchemaRows:   s.Int(ctx, KeyDBViewerSchemaRows),
		SQLBytes:     s.Int(ctx, KeyDBViewerMaxSQL),
	}
}

// LogRuleLimits returns the live log rule bounds.
func (s *Service) LogRuleLimits(ctx context.Context) LogRuleLimits {
	return LogRuleLimits{
		EvalEvery: s.Duration(ctx, KeyLogRuleEvalInterval),
		MaxWindow: s.Duration(ctx, KeyLogRuleMaxWindow),
		MaxRows:   s.Int(ctx, KeyLogRuleMaxRows),
		Samples:   s.Int(ctx, KeyLogRuleSamples),
	}
}
