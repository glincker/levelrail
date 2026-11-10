package runtimesettings

// Statement timing keys for the control plane's own database.
const (
	KeyPerfStatementTiming = "perf.statement_timing"
	KeyPerfSlowStatementMS = "perf.slow_statement_ms"
	KeyPerfSlowStatements  = "perf.slow_statement_ring"
)

func init() {
	Register(Definition{
		Key: KeyPerfStatementTiming, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_PERF_STATEMENT_TIMING", Apply: ApplyLive,
		Title:       "Database statement timing",
		Description: "Time every statement the control plane runs against its own database, count them per request, send a Server-Timing header to admins and keep the slowest statements. Off costs nothing. The next request uses a changed value.",
	})
	registerLimit(Definition{
		Key: KeyPerfSlowStatementMS, Category: CategoryObservability, Default: "100", Env: "APP_PERF_SLOW_STATEMENT_MS", Apply: ApplyLive,
		Title:       "Slow statement threshold (ms)",
		Description: "A statement at or above this many milliseconds is logged (statement text only, never values) and kept in the slowest statements list, from 1 to 60000.",
	}, 1, 60000)
	registerLimit(Definition{
		Key: KeyPerfSlowStatements, Category: CategoryObservability, Default: "20", Env: "APP_PERF_SLOW_STATEMENT_RING", Apply: ApplyLive,
		Title:       "Slowest statements kept",
		Description: "How many distinct slow statements the control plane remembers, from 1 to 500.",
	}, 1, 500)
}
