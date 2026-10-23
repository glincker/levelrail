---
title: "Traffic export"
description: Stream one record per finished edge request to PostHog, an OpenTelemetry collector or your own webhook, without visitor identity by default and without ever slowing a request.
"og:url": "https://docs.levelrail.com/traffic-export"
---

Traffic export copies each finished request at the ingress to an analytics tool you already use. Nothing is exported until you add a destination in Settings, Traffic export, or with the CLI.

## What a record contains

Every record carries the time, app, host, method, path, status, duration, bytes in and out, request id, trace id, whether the upstream failed and whether the request woke a sleeping app. It never carries cookies, auth headers, query strings or request bodies.

## Visitor identity is opt in

By default a record has no client IP, user agent, visitor id or referrer. Each destination can opt into any of these fields:

| Field | Meaning |
| --- | --- |
| `referrer_host` | Host of the Referer header, nothing else |
| `client_ip` | Client address, truncated by default (last octet of IPv4, last 80 bits of IPv6) |
| `user_agent` | The user agent string |
| `visitor_id` | A hash of IP and user agent that rotates daily and cannot be matched across days |

These can identify a person. Turn them on only if your privacy notice covers the destination. The dashboard shows a warning while any identity field is ticked, and the CLI prints one.

## Destinations

| Kind | Credentials |
| --- | --- |
| PostHog | Project token, plus a region (`us`, `eu`) or a self-hosted base URL |
| OpenTelemetry | Logs endpoint URL, optional Authorization header value |
| Webhook | Endpoint URL, optional Authorization header value (newline-delimited JSON) |

Credentials are stored encrypted per destination. The API and dashboard only report whether one is set, and they are never logged. The CLI reads them from stdin or a hidden prompt, never from arguments.

```bash
printf '%s\n' "$POSTHOG_TOKEN" | levelrail-cli traffic export sinks add \
  --kind posthog --name "Product analytics" --host eu --secret-stdin
levelrail-cli traffic export sinks test <id>
levelrail-cli traffic export sinks status
```

## Delivery guarantees

Records go into a bounded queue per destination, are batched and retried with backoff, and are dropped and counted when the queue is full. Export runs off the request path: a slow or failing destination never delays or fails a request. Status shows the last success, the last error kind (`auth`, `server`, `timeout` and so on, never a message or URL), and the queued, sent, dropped and failed counters.

Private and loopback targets are refused unless `APP_TRAFFIC_EXPORT_ALLOW_PRIVATE=1`. Tuning variables: `APP_TRAFFIC_EXPORT_QUEUE_SIZE`, `APP_TRAFFIC_EXPORT_BATCH_SIZE`, `APP_TRAFFIC_EXPORT_FLUSH_INTERVAL`, `APP_TRAFFIC_EXPORT_MAX_ATTEMPTS`, `APP_TRAFFIC_EXPORT_BACKOFF_BASE`, `APP_TRAFFIC_EXPORT_BACKOFF_MAX`, `APP_TRAFFIC_EXPORT_TIMEOUT` and `APP_TRAFFIC_EXPORT_MAX_BODY_BYTES`.

## API

Sinks live under `/api/v1/settings/traffic-export/sinks`, with `POST .../sinks/{id}/test` for one synthetic record and `GET /api/v1/settings/traffic-export/status`. Reading needs the read ability, writing and testing need root. See the [API reference](/api-reference).
