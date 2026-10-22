---
title: "Traffic observability: rate, errors and duration"
description: Per app and per replica request rate, error rate and p50/p95/p99 latency measured at the ingress, with slow request samples and SLO error-budget burn.
"og:url": "https://docs.levelrail.com/traffic-observability"
---

Every app that serves traffic through a domain gets request rate, error rate and latency percentiles with no setup and no change to the app. The numbers are measured at the embedded ingress, kept on the node that served the requests, and queried on demand.

## What you see

Open an app and choose **Traffic** under **Observe**, or run `traffic <app>` in the CLI. For the window you pick (5 minutes, 1 hour, 24 hours or 7 days):

| Figure | Meaning |
| --- | --- |
| Rate | Requests per second |
| Errors | 4xx and 5xx share of requests, and a count by status class |
| Duration | p50, p95 and p99 latency |
| Replicas | The same figures per upstream address, so one bad container stands out |
| Top paths | The busiest route templates by request count, with 4xx and 5xx share |
| Slowest routes | Route templates ranked by average duration |
| Error spikes | Stretches where the 5xx share stood out, each matched to the deploy that started shortly before |
| Slow and failed requests | A sample of 5xx, upstream failures and slow requests |
| Error budget | Burn rate and budget remaining when the app has an SLO alert |

## Bounded by design

Memory and storage stay flat as traffic grows.

- Latency uses a fixed 12 bucket histogram per app and per replica. Percentiles are estimated from the buckets, so they are accurate to the bucket width.
- Routes are normalized templates such as `/api/users/:id`. Ids, tokens, query strings and client addresses are never read or stored. The number of distinct routes and replicas kept per tick is capped, and the rest fall into an `other` bucket.
- The sample log keeps failed requests (5xx and upstream failures) and a fraction of slow ones, capped per tick and per app, and expires with a retention period.

## Settings

All settings are environment variables on the control plane or agent.

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_TRAFFIC_RETENTION` | metrics minute tier (`APP_METRICS_RETENTION_1M`, 30d) | How long per-minute traffic aggregates (routes, replicas, audience) are kept |
| `APP_REQUEST_SLOW_MS` | `1000` | Duration at which a request counts as slow |
| `APP_REQUEST_SAMPLE_RATE` | `0.1` | Fraction of slow requests sampled. Failed requests are always eligible |
| `APP_REQUEST_SAMPLE_TICK_MAX` | `50` | Samples kept per app per sampling tick. `0` turns sampling off |
| `APP_REQUEST_SAMPLE_ROWS` | `5000` | Samples stored per app |
| `APP_REQUEST_SAMPLE_RETENTION` | `24h` | How long samples are kept |
| `APP_REQUEST_UPSTREAM_MAX` | `32` | Distinct replica labels kept per tick |
| `APP_REQUEST_ROUTE_MAX` | `50` | Distinct routes kept per tick |
| `APP_TRAFFIC_SLOW_ROUTE_MIN` | `5` | Requests a route needs before it can rank as slow |
| `APP_ALERT_REQUEST_WINDOW` | `5m` | Trailing window of the request alert metrics |
| `APP_TRAFFIC_SPIKE_MIN_RATE` | `0.05` | Smallest 5xx share (0 to 1) of a bucket that can count as a spike |
| `APP_TRAFFIC_SPIKE_FACTOR` | `3` | How many times the window median a bucket must exceed |
| `APP_TRAFFIC_SPIKE_MIN_REQUESTS` | `20` | Requests a bucket needs before it can count as a spike |
| `APP_TRAFFIC_SPIKE_ATTRIBUTION` | `15m` | How long after a deploy starts a spike is still blamed on it |
| `APP_TRAFFIC_SPIKE_MAX` | `10` | Spikes returned per window, newest first |
| `APP_TRAFFIC_SPIKE_BUCKETS` | `60` | Buckets the window is cut into for detection |

## Error spikes and deploys

The window is cut into buckets (`APP_TRAFFIC_SPIKE_BUCKETS`, never finer than 15 seconds). A run of consecutive buckets is a spike when each bucket has at least `APP_TRAFFIC_SPIKE_MIN_REQUESTS` requests and a 5xx share of at least `APP_TRAFFIC_SPIKE_MIN_RATE` and `APP_TRAFFIC_SPIKE_FACTOR` times the window median, so an app that always errors a little does not light up. Each spike is tied to the latest deploy that started no more than `APP_TRAFFIC_SPIKE_ATTRIBUTION` before it. The match is a hint, not proof: check the deploy before rolling back.

Spikes appear in the Traffic tab, in `traffic <app>` and in the `error_spikes` field of the API and the `get_app_traffic` MCP tool. They are computed on request from the stored per-minute counters, so nothing extra is stored.

## Alerts

A threshold alert rule can watch three request metrics, so "error rate above X for Y" and "p99 above X for Y" need no new rule type:

| Metric | Value |
| --- | --- |
| `request_error_rate_5xx_pct` | 5xx share of requests, as a percentage |
| `request_p95_ms` | p95 latency in milliseconds |
| `request_p99_ms` | p99 latency in milliseconds |

A window with no requests neither fires nor resolves a rule. For error-budget alerting use an SLO burn rule, which this page's budget figure reads.

## API

`GET /api/v1/apps/{name}/traffic?window=5m|1h|24h|7d` returns the totals, `status_classes`, `latency`, `replicas`, `slow_routes`, `top_routes`, `error_spikes`, `samples` and, when an SLO burn rule exists, `slo`. It needs read access to the app. Time series for the charts come from `GET /api/v1/apps/{name}/requests`.

## CLI

```
traffic <app> [--window|--since 5m|1h|24h|7d] [--watch] [--interval 5s] [--json]
```

With `--watch --json` each refresh is printed as one JSON line.

## Measured overhead

The ingress handler does one map lookup and a few counter increments per request under a short lock, and nothing is written to disk on the request path. Aggregates are drained once per tick into the node-local store. `BenchmarkRequestStatsObservePath` in `internal/ingress` measured about 410 ns and 4 allocations per request with 14 goroutines contending on one app, on an Apple M4 Max (one run, not a Linux VPS). Memory is bounded by the replica and route caps above. End to end proxy overhead under load has not been measured yet.

## Limits

- Replica figures need the request to pass through a reverse proxy upstream. Static sites and requests answered by the ingress itself have no replica.
- Percentiles are histogram estimates. A p99 above 10 seconds reports the last bucket bound.
- Spike detection reads per-bucket counters, so a spike shorter than a bucket is averaged out. Use a shorter window to see it.
- The MCP server returns the same report through the `get_app_traffic` tool.
