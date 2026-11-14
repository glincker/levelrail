package runtimesettings

// Keys for the OTLP logs and metrics receiver. It is off until an operator turns it on.
const (
	KeyOTLPIngestEnabled    = "otlp.ingest_enabled"
	KeyOTLPIngestMaxBody    = "otlp.ingest_max_body_bytes"
	KeyOTLPIngestMaxRecords = "otlp.ingest_max_records"
	KeyOTLPIngestMaxLine    = "otlp.ingest_max_line_bytes"
)

func init() {
	Register(Definition{
		Key: KeyOTLPIngestEnabled, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_OTLP_INGEST_ENABLED", Apply: ApplyLive,
		Title:       "OTLP logs and metrics ingest",
		Description: "Accept OTLP/HTTP logs and metrics at /api/v1/otlp/v1/logs and /v1/metrics from apps that have tracing enabled, using the same per-app key. Traces are always accepted separately. Off by default.",
	})
	registerLimit(Definition{
		Key: KeyOTLPIngestMaxBody, Category: CategoryObservability, Default: "4194304", Env: "APP_OTLP_INGEST_MAX_BODY_BYTES", Apply: ApplyLive,
		Title:       "OTLP ingest: body size",
		Description: "Largest decompressed OTLP logs or metrics request in bytes. From 64 KiB to 64 MiB. Bigger requests get 413.",
	}, 64<<10, 64<<20)
	registerLimit(Definition{
		Key: KeyOTLPIngestMaxRecords, Category: CategoryObservability, Default: "5000", Env: "APP_OTLP_INGEST_MAX_RECORDS", Apply: ApplyLive,
		Title:       "OTLP ingest: records per request",
		Description: "Most log records or metric points one request may carry. From 100 to 100000. A request over the limit is rejected whole with 413.",
	}, 100, 100_000)
	registerLimit(Definition{
		Key: KeyOTLPIngestMaxLine, Category: CategoryObservability, Default: "8192", Env: "APP_OTLP_INGEST_MAX_LINE_BYTES", Apply: ApplyLive,
		Title:       "OTLP ingest: log line size",
		Description: "Longest stored log message in bytes, longer ones are cut. From 256 to 65536.",
	}, 256, 65536)
}
