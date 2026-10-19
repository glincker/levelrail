---
title: "Data export"
description: Send logs, the audit log and metrics to Loki, syslog, a webhook, a bucket, Prometheus or an OTLP collector, with bounded buffers and no effect on deploys.
"og:url": "https://docs.levelrail.com/data-export"
---

Metrics, logs and the audit log stay on the control plane and its nodes by default. Data export sends copies to the tools you already run: Grafana and Prometheus, Loki, a syslog server, a SIEM webhook, an S3 compatible bucket, or an OpenTelemetry collector. Nothing is exported until you add a destination, and a destination that is down never slows a container, a deploy or the API.

## What you can export

| Signal | Destination type | How it leaves |
| --- | --- | --- |
| Metrics | Prometheus scrape | You pull `GET /metrics` |
| Metrics | `metrics_otlp` | Pushed as OTLP/HTTP JSON on an interval |
| Metrics | `metrics_remote_write` | Pushed as Prometheus remote write on an interval |
| Logs | `logs_loki` | Loki push API, batched |
| Logs | `logs_otlp` | OTLP/HTTP JSON logs, batched |
| Logs | `logs_http` | JSON lines (NDJSON) over HTTP POST, batched |
| Logs | `logs_syslog` | RFC 5424 over TCP or TLS |
| Logs | `logs_newrelic`, `logs_datadog`, `logs_axiom`, `logs_betterstack` | The vendor's own ingest API, you supply only a key |
| Audit log | `audit_webhook` | NDJSON batches over HTTP POST, optional HMAC signature |
| Audit log | `audit_s3` | NDJSON objects in a connected storage destination |
| Audit log | download | `GET /api/v1/audit-log/export`, NDJSON or CSV |

Manage destinations from **Settings, Data export** in the dashboard or with `levelrail-cli export destinations`.

## Prometheus scrape

`GET /metrics` returns the Prometheus text format. It is gated like every read route: give Prometheus a token with the `read` ability as a bearer credential.

```yaml
scrape_configs:
  - job_name: levelrail
    scheme: https
    metrics_path: /metrics
    authorization:
      credentials_file: /etc/prometheus/levelrail.token
    static_configs:
      - targets: ['levelrail.example.com']
```

Cardinality is bounded. The only labels are `app`, `database` and `node` (one per series, from the resource the sample belongs to) and `destination` on the exporter's own series, so the series count tracks what you have deployed and what you configured, not request traffic.

| Metric | Type | Labels |
| --- | --- | --- |
| `<prefix>cpu_percent` | gauge | `app`, `database` or `node` |
| `<prefix>memory_usage_bytes`, `<prefix>memory_limit_bytes` | gauge | same |
| `<prefix>network_rx_bytes`, `<prefix>network_tx_bytes` | counter | same |
| `<prefix>disk_read_bytes`, `<prefix>disk_write_bytes` | counter | same |
| `<prefix>apps` | gauge | none |
| `<prefix>export_sent_total`, `_dropped_total`, `_failures_total` | counter | `destination` |
| `<prefix>export_queue_depth`, `_circuit_open` | gauge | `destination` |

The prefix is the lowercase brand short name followed by an underscore, `levelrail_` on a stock install. A token limited to some apps only sees those apps and databases, and no node or platform series.

## Loki and Grafana quick start

1. Run Loki and Grafana wherever you like. For a trial on one host:

   ```bash
   docker network create obs
   docker run -d --name loki --network obs -p 3100:3100 grafana/loki:3.3.2
   docker run -d --name grafana --network obs -p 3000:3000 grafana/grafana:11.3.0
   ```

2. Loki on the same host or a private network is an internal address, which export refuses by default. On the control plane set `APP_EXPORT_ALLOW_PRIVATE_NETWORKS=true` and restart (see [Environment variables](/environment-variables#data-export)). A public HTTPS Loki needs nothing.
3. Add the destination:

   ```bash
   levelrail-cli export destinations create --name loki --kind logs_loki \
     --endpoint http://loki.internal:3100 --app web --min-level info \
     --label env=prod
   levelrail-cli export destinations test <id>
   ```

   For Grafana Cloud or a multi-tenant Loki add `--header X-Scope-OrgID=tenant` or `--header "Authorization=Basic <base64>"`. Header values are stored encrypted and are never returned by the API.
4. In Grafana add a Loki data source pointing at `http://loki:3100`, then query `{job="levelrail", app="web"}`.

Loki streams carry the labels `job`, `app`, `kind` (`app` or `database`), `stream` (`stdout` or `stderr`) and `level`, plus any `--label` you set. `level` is the structured `level` or `severity` field when the line is JSON, otherwise `error` for stderr and `info` for stdout.

## Log destinations

Filters apply per destination: `--app` (repeatable, empty means every app) and `--min-level` (`debug`, `info`, `warn`, `error`). Database containers are exported only by destinations with no `--app` filter.

- HTTP JSON lines: one JSON object per line: `time`, `kind`, `app`, `stream`, `level`, `message` and `fields` for structured lines. `Content-Type: application/x-ndjson`.
- Loki: an endpoint with no path gets `/loki/api/v1/push` appended.
- Syslog: `tcp://host:port` or `tls://host:port`, RFC 5424 with octet counting framing, facility `local0`. TLS verifies the server certificate against the system roots.

Log export reuses the log stream the live tail already reads: it adds a subscriber, never a second Docker reader.

## Vendor destinations

Four vendors have a destination kind of their own, so you paste a key and pick a region instead of working out an endpoint and a header. They use the same queue, retries, circuit breaker, filters, status and test as every other log destination.

| Kind | Endpoint it uses | Key header | Options |
| --- | --- | --- | --- |
| `logs_newrelic` | `https://log-api.newrelic.com/log/v1`, or `log-api.eu.newrelic.com` | `Api-Key` | `region` us or eu |
| `logs_datadog` | `https://http-intake.logs.<site>/api/v2/logs` | `DD-API-KEY` | `site`, default `datadoghq.com` |
| `logs_axiom` | `https://api.axiom.co/v1/datasets/<dataset>/ingest`, or `api.eu.axiom.co` | `Authorization: Bearer` | `dataset` (required), `region` |
| `logs_betterstack` | `https://in.logs.betterstack.com` | `Authorization: Bearer` | none; use `--endpoint` for a source specific host |

The key is stored encrypted like any export header and never returned; a destination shows `has_api_key` instead. An explicit `--endpoint` overrides the preset, for a proxy or a regional host. Changing `region`, `site` or `dataset` on a preset destination moves its endpoint with it.

```bash
export NR_KEY=...
levelrail-cli export destinations create --name new-relic --kind logs_newrelic \
  --api-key-env NR_KEY --region eu --app web --min-level info
levelrail-cli export destinations test <id>
```

Each line is sent with the app name as the service, its stream and its level. For structured lines the parsed fields are included (New Relic as `fields`, Axiom flattened into the record).

## Audit log export

Streaming sends each new audit row at least once. Each batch is NDJSON, and webhook batches carry these headers (the prefix follows the brand name):

| Header | Meaning |
| --- | --- |
| `X-Levelrail-Batch-First-Id`, `X-Levelrail-Batch-Last-Id`, `X-Levelrail-Batch-Count` | Row ids, so a receiver can deduplicate |
| `X-Levelrail-Signature` | `t=<unix>,v1=<hex>` when a signing key is set |

The signature is HMAC-SHA256 over `<unix>.<raw body>` with your key. Verify it and reject timestamps outside a few minutes:

```python
import hmac, hashlib, time

def verify(key: bytes, header: str, body: bytes, tolerance=300) -> bool:
    parts = dict(p.split("=", 1) for p in header.split(","))
    expected = hmac.new(key, parts["t"].encode() + b"." + body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, parts["v1"]) and abs(time.time() - int(parts["t"])) <= tolerance
```

`audit_s3` writes each batch to `<prefix>/<yyyy>/<mm>/<dd>/<first-created-at>-<first-id>.ndjson` in a [storage destination](/object-storage). The key is derived from the batch's first row, so a retried batch overwrites itself.

Tokens, bearer values, URL credentials and token-shaped strings are masked in every exported field. By default streaming starts at the moment the destination is created. `--backfill` starts at the oldest audit row instead.

### Delivery semantics

- A cursor (timestamp and row id) is saved after every successful batch, so a restart resumes where it stopped.
- Delivery is at least once. A crash between a receiver accepting a batch and the cursor being saved sends that batch again, with the same id range.
- A failing destination holds the cursor and retries. Rows are never skipped, and the audit log itself is the buffer.

### On demand download

```bash
levelrail-cli export audit --from 2026-10-01T00:00:00Z --to 2026-10-08T00:00:00Z --format csv > audit.csv
```

`from` is inclusive and `to` exclusive. The API route is `GET /api/v1/audit-log/export?from=&to=&format=ndjson|csv` and needs the `root` ability. A single download is capped at `APP_EXPORT_AUDIT_MAX_ROWS`.

## OTLP metrics

A `metrics_otlp` destination pushes the same series as the scrape endpoint as OTLP/HTTP JSON (`/v1/metrics` is appended when the endpoint has no path). Counters are cumulative monotonic sums, everything else gauges. The interval defaults to `APP_EXPORT_METRICS_INTERVAL` and can be set per destination with `--interval` (10 to 3600 seconds).

```bash
levelrail-cli export destinations create --name otel --kind metrics_otlp \
  --endpoint https://otel.example.com:4318 --header "Authorization=Bearer <token>"
```

A `metrics_otlp` destination also accepts `--app` (repeatable) to export only those apps' container series. Platform series are always included.

### Prometheus remote write

A `metrics_remote_write` destination pushes the same series as the scrape endpoint using Prometheus remote write 1.0 (snappy compressed protobuf), so Grafana Cloud, Mimir, VictoriaMetrics and Prometheus started with `--web.enable-remote-write-receiver` can ingest it. The endpoint is used as given, because the path differs per receiver (for example `/api/v1/write`, `/api/v1/push`, `/api/prom/push`). Series names and labels match the scrape endpoint, and the interval works like `metrics_otlp`.

Pass `--username` with `--api-key-env` for basic auth (Grafana Cloud uses the instance id and an access policy token). Without a username the key is sent as a bearer token. `--header` can set any other auth header. Secrets are stored encrypted and never returned. `--app` limits the container series to those apps.

```sh
GRAFANA_TOKEN=glc_xxx levelrail-cli export destinations create --name grafana --kind metrics_remote_write \
  --endpoint https://prometheus-prod-01.grafana.net/api/prom/push \
  --username 123456 --api-key-env GRAFANA_TOKEN
```

### Platform self metrics

Besides per-app container metrics, the scrape endpoint and every `metrics_otlp` destination carry a fixed set of control plane gauges (prefix is the brand short name, `levelrail_` on a stock install). Restricted tokens never see them.

| Metric | Meaning |
| --- | --- |
| `<prefix>reconcile_lag_seconds` | Age of the stalest reconcile status. A growing value means a controller stopped converging |
| `<prefix>deploy_queue_depth`, `<prefix>deploys_running` | Deploys waiting and in flight |
| `<prefix>deploy_last_duration_seconds`, `<prefix>deploy_avg_duration_seconds` | Duration of the newest and the mean of deploys finished in the last hour |
| `<prefix>deploys_finished{status}` | Deploys finished in the last hour by `succeeded` or `failed` |
| `<prefix>agent_connections` | Nodes whose agent is online |

## OTLP logs

A `logs_otlp` destination pushes container log lines as OTLP/HTTP JSON (`/v1/logs` is appended when the endpoint has no path). Each app is one resource with `service.name` set to the app name, `kind`, and your `--label` pairs as resource attributes. Records carry the severity, the stream and, for JSON lines, the parsed fields. `--app` and `--min-level` filter as for the other log kinds.

```bash
levelrail-cli otel destinations add --name otel-logs --signal logs \
  --endpoint https://otel.example.com:4318 --header "Authorization=Bearer <token>" --app web
levelrail-cli otel destinations test <id>
levelrail-cli otel destinations list
```

`levelrail-cli otel destinations` is a signal-first view of the same destinations: `add --signal metrics|logs` creates a `metrics_otlp` or `logs_otlp` kind, and `remove` deletes one. Delivery state (last success, last error, queue depth, dropped) is the same as for every other destination, in **Settings, Data export**.

## Reliability and safety

- Every log destination has a queue of `APP_EXPORT_QUEUE_LINES` entries. When it fills, the oldest entries are dropped and counted.
- The queue is mirrored on disk under `APP_EXPORT_SPOOL_DIR` (default `<data dir>/export-spool`), so lines a down receiver has not accepted survive a control plane restart and are sent after it. A batch that fails after its retries stays queued and is tried again, instead of being dropped. A crash between a send and its record of success can deliver that batch twice, so delivery is at least once. Set `APP_EXPORT_SPOOL=false` to keep the backlog in memory only. Audit rows never needed this: they are read from the audit log by cursor.
- Container log lines are handed to exporters through a non-blocking subscription. A slow destination cannot slow a container, the log store, a deploy or the API.
- A batch is retried with exponential backoff and jitter up to `APP_EXPORT_MAX_ATTEMPTS` times. Responses in the 4xx range other than 408 and 429 are not retried, because retrying cannot fix them.
- After `APP_EXPORT_BREAKER_FAILURES` consecutive failed batches a destination backs off for `APP_EXPORT_BREAKER_COOLDOWN`, then tries one batch.
- Endpoints are validated when saved and again at every connection, including redirects: private, loopback and link-local addresses are refused unless `APP_EXPORT_ALLOW_PRIVATE_NETWORKS=true`. Endpoints cannot embed credentials.
- Headers and signing keys are stored with envelope encryption and are never returned. The API shows header names and whether a key is set.
- Each destination shows its last success, last error, queue depth, dropped and failed counts, and breaker state, in the dashboard, with `levelrail-cli export destinations status <id>`, and as `export_*` metrics.

## CLI

```
levelrail-cli export destinations list
levelrail-cli export destinations create --name N --kind K [--endpoint URL] [--api-key-env VAR] [--region us|eu] [--site HOST] [--dataset NAME] [--app A] [--min-level L] [--header K=V] [--hmac-key KEY] [--storage-target ID] [--label K=V] [--interval S] [--backfill] [--disabled]
levelrail-cli export destinations test <id>
levelrail-cli export destinations status <id>
levelrail-cli export destinations enable|disable <id>
levelrail-cli export destinations delete <id>
levelrail-cli export audit [--from T] [--to T] [--format ndjson|csv]
```

`--header` and `--hmac-key` values end up in your shell history. Prefer a script that reads them from a secret store.

## Next steps

<CardGroup cols={2}>
<Card title="Observability" href="/observability">

Node-local metrics and the remote read endpoint.

</Card>
<Card title="Log archive" href="/log-archive">

Scheduled, compressed log archives in a bucket.

</Card>
</CardGroup>
