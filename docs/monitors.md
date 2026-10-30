---
title: "Monitors"
description: Experimental http and tcp uptime monitors with a bounded result history and hourly rollups, off unless APP_MONITORS_ENABLED is set.
"og:url": "https://docs.levelrail.com/monitors"
---

<Warning>Monitors are experimental. The API, CLI and storage may change between releases.</Warning>

A monitor probes an endpoint on a schedule from the control plane and records the result. This first slice has `http` and `tcp` monitors, a result history, and hourly rollups. It does not send alerts, open incidents or change the status page.

The feature is off by default. When `APP_MONITORS_ENABLED` is not true, the `/api/v1/monitors` routes answer 404 and no scheduler runs.

## Kinds

- `http`: method (`GET`, `HEAD`, `POST`, `PUT`), optional headers, accepted status codes or ranges (default 200 to 399), an optional `contains` keyword searched in the first bytes of the body, and a timeout. Redirects are not followed unless asked for, up to a server limit.
- `tcp`: a host and port. The check passes when the connection completes within the timeout.

Header values are stored encrypted with the same secret store as app secrets. They are never returned by the API or the CLI, and results keep only a short failure class such as `timeout` or `status_mismatch`, never a body or header value.

## Network safety

Every connection, including redirects, goes through the outbound address guard. Private, loopback and link local addresses are refused unless the control plane has explicitly allowed private outbound targets. A refused target shows as `blocked_address` and is rejected at create time.

## Results

Results are written in batches, with the hourly rollup updated in the same transaction. Raw results are kept for a short window and rollups for much longer. Pruning runs in small batches so it does not hold the database write lock.

```
levelrail-cli monitors create --name "Site" --kind http --url https://example.com/health
levelrail-cli monitors results mon_1a2b3c4d5e6f7a8b --window 6h
```

## Settings

All limits are environment variables on the control plane. Durations use Go syntax such as `30s` or `48h`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MONITORS_ENABLED` | `false` | Turn the feature on |
| `APP_MONITOR_WORKERS` | `4` | Concurrent probes |
| `APP_MONITOR_MIN_INTERVAL_HTTP` | `30s` | Lowest http interval |
| `APP_MONITOR_MIN_INTERVAL_TCP` | `30s` | Lowest tcp interval |
| `APP_MONITOR_MAX_TIMEOUT` | `30s` | Highest per check timeout |
| `APP_MONITOR_MAX_COUNT` | `100` | Monitors per control plane |
| `APP_MONITOR_BODY_MAX_BYTES` | `65536` | Body bytes searched for `contains` |
| `APP_MONITOR_HEADER_MAX_COUNT` | `16` | Headers per monitor |
| `APP_MONITOR_HEADER_MAX_BYTES` | `4096` | Bytes per header value |
| `APP_MONITOR_MAX_REDIRECTS` | `5` | Redirect hops when following |
| `APP_MONITOR_FLUSH_INTERVAL` | `5s` | How often results are written |
| `APP_MONITOR_BUFFER_MAX` | `10000` | Results held in memory, oldest dropped first |
| `APP_MONITOR_RAW_RETENTION` | `48h` | Raw result retention |
| `APP_MONITOR_HOUR_RETENTION` | `9600h` | Hourly rollup retention |
| `APP_MONITOR_PRUNE_INTERVAL` | `10m` | How often pruning runs |
| `APP_MONITOR_PRUNE_BATCH` | `1000` | Rows deleted per pruning step |
| `APP_MONITOR_JITTER_PERCENT` | `10` | Start spread as a share of the interval |
| `APP_MONITOR_TICK_INTERVAL` | `1s` | Scheduler resolution |
| `APP_MONITOR_RESULTS_MAX_LIMIT` | `1000` | Most raw results one request returns |

## API

- `GET /api/v1/monitors`, `POST /api/v1/monitors`, `DELETE /api/v1/monitors/{id}`
- `GET /api/v1/monitors/{id}/results?window=24h&limit=100` returns recent raw results and hourly rollups.

Reading needs read access and changing needs write access on the `monitors` resource. Creating and deleting are recorded in the audit log.
