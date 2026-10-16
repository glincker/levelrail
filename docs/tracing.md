---
title: "Tracing"
description: Receive OpenTelemetry traces from your apps, keep them on the node and read them as a waterfall in the dashboard.
---

Your apps send spans to the control plane over OTLP/HTTP, and the dashboard shows each trace as a waterfall next to the app's logs and metrics. Nothing is shipped anywhere else, and nothing is stored until you turn tracing on for an app.

## Turn it on

In the dashboard, open the app, then **Observe, Traces**, and choose **Turn on tracing**. Or:

```bash
levelrail-cli apps tracing enable web
```

You get an ingest key and three environment variables. The key is shown once:

```
OTEL_EXPORTER_OTLP_ENDPOINT=https://cp.example.com/api/v1/otlp
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer <key>
OTEL_SERVICE_NAME=web
```

Set them on the app, redeploy, and use any OpenTelemetry SDK with its OTLP/HTTP exporter (`http/protobuf` or `http/json`). gRPC is not accepted on this endpoint.

The endpoint host is the one you used to reach the control plane. If your containers reach it under another name, replace the host in `OTEL_EXPORTER_OTLP_ENDPOINT`.

## What is stored

| Field | Source |
| --- | --- |
| Trace and span ids, parent | The span |
| Name, kind, start and end time | The span |
| Status and message | The span (`error` marks the trace as failed) |
| Attributes | The span's attributes, plus `service.name` from the resource. Values are cut at 1 KiB |

Spans belong to the app whose key sent them, whatever `service.name` says. One export may carry up to 5000 spans and 8 MiB (after gzip). Spans age out with the logs retention window, `APP_LOGS_RETENTION`.

## Read them

- **Dashboard**: Observe, Traces lists traces newest first, filters by window, minimum duration and errors only, and opens a waterfall.
- **CLI**: `levelrail-cli apps traces list web --errors --min-ms 250`, then `levelrail-cli apps traces get web <trace-id>` for the tree.
- **API**: `GET /api/v1/apps/{name}/traces` and `GET /api/v1/apps/{name}/traces/{id}`.

## Keys

| Action | Command |
| --- | --- |
| Create or rotate | `levelrail-cli apps tracing enable <app>` |
| Check | `levelrail-cli apps tracing status <app>` |
| Turn off | `levelrail-cli apps tracing disable <app>` |

A key works for exactly one app and can only write spans. It is stored encrypted, a rotation takes effect within a minute, and turning tracing off makes the app's exporter get `401`.

## Limits

- Traces only. Logs and metrics are exported with [data export](data-export.md).
- Spans are stored on the control plane node, not federated across agents yet.
- There is no sampling on this side: set a sampler in the SDK if volume is high.

## Troubleshooting

### The exporter gets 401

The key is wrong, was rotated or tracing was turned off. Run `levelrail-cli apps tracing enable <app>` and set the new header.

### The exporter gets 415 or 400

The endpoint only accepts OTLP over HTTP with protobuf or JSON. Set `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`, not `grpc`.

### Traces list is empty

Widen the window, check the app is sending (`OTEL_EXPORTER_OTLP_ENDPOINT` is set and the container was redeployed), and check the exporter is not failing on a network name the container cannot resolve.

## Edge requests and correlation ids

Every request through the edge gets an `X-Request-Id` and a W3C `traceparent`
header before it reaches your app. A value the client already sent is kept, so
an upstream proxy or browser SDK stays in the same trace. The request id is
also returned on the response.

The control plane keeps the most recent requests per app in memory (default
1000 per route, `APP_ACCESS_LOG_SIZE`, `0` turns it off). A restart clears
them. Query strings and client addresses are never recorded.

```sh
levelrail-cli apps access web --slow-ms 500
levelrail-cli apps access web --min-status 500 --path /api
```

The same list is under **Requests** in the app's Observe menu and at
`GET /api/v1/apps/{name}/access` (`min_ms`, `min_status`, `path`, `since`,
`limit`). Each row carries the request id and trace id, so a slow request can
be matched to the span tree on the Traces page.
