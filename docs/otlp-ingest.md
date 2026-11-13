---
title: "OTLP logs and metrics ingest"
description: Send OpenTelemetry logs and metrics from your apps to the control plane over OTLP/HTTP, next to traces.
"og:url": "https://docs.levelrail.com/otlp-ingest"
---

Apps that already send [traces](/tracing) can also send logs and metrics to the same endpoint family. Logs land in the app's log store and metrics land as app metrics. It is off until an admin turns it on.

## Turn it on

Set the `otlp.ingest_enabled` runtime setting to `true` (Settings, Runtime, Observability, or `levelrail-cli settings runtime set otlp.ingest_enabled true`). Tracing must be enabled for the app, because the same per-app bearer key authorizes the request. While the setting is off, both endpoints answer `501`. A missing or wrong key answers `401`.

## Endpoints

| Endpoint | Carries |
| --- | --- |
| `POST /api/v1/otlp/v1/logs` | OTLP logs |
| `POST /api/v1/otlp/v1/metrics` | OTLP metrics |

The body is OTLP protobuf (`application/x-protobuf`) or JSON (`application/json`), optionally gzip compressed with `Content-Encoding: gzip`. Any other content type answers `415`.

## Limits

All three apply live and are runtime settings. A request over the body or record limit is rejected whole with `413`.

| Setting | Meaning |
| --- | --- |
| `otlp.ingest_max_body_bytes` | Largest decompressed request body. |
| `otlp.ingest_max_records` | Most log records or metric points in one request. |
| `otlp.ingest_max_line_bytes` | Longest stored log message, longer ones are cut. |

## What is mapped

- Logs: the record body becomes the log line for the app. Records with severity error or higher (17 and up) are stored on the error stream, the rest on stdout. Empty bodies are skipped.
- Metrics: gauges, sums and histograms are stored as `otlp.<name>`. A histogram becomes `otlp.<name>.count` and `otlp.<name>.sum`.
- Attributes are dropped. Two series that differ only by attributes share one name, and the later point wins.

## Not supported

- gRPC. Use `http/protobuf` or `http/json`.
- zstd compression. Only gzip is decoded.
- Exponential histograms and summaries. They are ignored.
- Attributes as separate series or labels.

## Send a test

```bash
curl -X POST "$CP/api/v1/otlp/v1/logs" \
  -H "Authorization: Bearer $INGEST_KEY" \
  -H "Content-Type: application/json" \
  -d '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"severityNumber":9,"body":{"stringValue":"hello from otlp"}}]}]}]}'
```

A `200` with `{}` means it was stored.

## Collector exporter

```yaml
exporters:
  otlphttp/levelrail:
    endpoint: https://cp.example.com/api/v1/otlp
    headers:
      Authorization: Bearer ${env:INGEST_KEY}
    encoding: proto
    compression: gzip

service:
  pipelines:
    logs:
      exporters: [otlphttp/levelrail]
    metrics:
      exporters: [otlphttp/levelrail]
```
