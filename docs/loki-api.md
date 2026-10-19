---
title: "Loki-compatible log API"
description: Read your app logs from Grafana Explore through a Loki-compatible API served by the control plane, with no log shipper in between.
"og:url": "https://docs.levelrail.com/loki-api"
---

The control plane answers a read-only subset of the Loki HTTP API over the node-local log store. Add it to a self-hosted Grafana as a Loki data source and use Explore to search and tail app logs. Nothing is copied into another store.

## Add the data source in Grafana

1. Create an API token with the `read` ability in the dashboard (**Settings, API tokens**).
2. In Grafana, open **Connections, Data sources, Add data source, Loki**.
3. Set **URL** to your control plane address plus `/api/v1/loki`, for example `https://levelrail.example.com/api/v1/loki`.
4. Turn on **Basic auth**. Use any user name and paste the token as the **password**. Alternatively add a custom header `Authorization` with the value `Bearer <token>`.
5. Click **Save & test**, then open **Explore** and run `{app="web"}`.

Apps the token cannot read are left out of results and out of the label values.

## Endpoints

Base path: `/api/v1/loki`. The paths Grafana builds (`/api/v1/loki/loki/api/v1/...`) are served too, so the data source URL above works unchanged.

| Method | Path | Notes |
| --- | --- | --- |
| `GET`, `POST` | `/query_range` | `query`, `start`, `end`, `limit`, `direction` |
| `GET`, `POST` | `/query` | Instant form: the last 5 minutes ending at `time` |
| `GET` | `/labels` | `app`, `level`, `stream` |
| `GET` | `/label/{name}/values` | Apps you can read, `stdout` and `stderr`, or the level names |
| `GET` | `/ready` | Answers `ready` |

All of them require the `read` ability. Times are unix seconds, unix nanoseconds or RFC3339. `limit` defaults to 100 and is capped at 5000. `direction` is `backward` (newest first, the default) or `forward`. A range may not exceed 30 days (`APP_LOKI_MAX_RANGE`), and at most `APP_LOG_SIGNATURE_MAX_ROWS` lines are scanned per app per query.

## Supported LogQL

```
{app="web"}
{app=~"web|api", stream="stderr", level!="debug"}
{app="web"} |= "timeout" != "healthz" |~ "status=5\\d\\d"
```

- Stream selector with `=`, `!=`, `=~` and `!~` on `app`, `stream` and `level`. At least one `app` matcher with `=` or `=~` is required. Regexes are fully anchored, as in Loki.
- Line filters `|=`, `!=`, `|~` and `!~`, chained in any order.
- `level` is present when a level could be read from the line or its JSON fields.

Anything else is refused with HTTP 400 and a Loki-style error body (`{"status":"error","errorType":"bad_data","error":"..."}`) instead of a guessed answer: metric queries (`rate`, `count_over_time`, `sum`), parsers and stages (`| json`, `| logfmt`, `| line_format`, `unwrap`), and other labels.

For counts and trends use the [log explorer](log-explorer) and the metrics API instead.
