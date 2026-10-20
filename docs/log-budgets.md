---
title: "Log volume budgets"
description: Cap how much log data one app can store, see which apps are noisiest, and forecast when a budget will be reached.
"og:url": "https://docs.levelrail.com/log-budgets"
---

Log retention is time based (`APP_LOGS_RETENTION`), so one chatty app can fill the disk or push everyone else's history out. A budget caps the stored log bytes per app. When an app goes over, its oldest lines are trimmed. Other apps are never touched.

## Set a budget

Every app can have its own override, and a fleet default applies to apps without one.

```bash
levelrail-cli apps log-budget set web --max-mb 512
levelrail-cli apps log-budget get web
levelrail-cli apps log-budget clear web
```

In the dashboard, open **Settings, Observability, Log volume** and click the pencil on an app's budget. The app's **Logs** page shows the same field.

| Setting | Default | Meaning |
| --- | --- | --- |
| `APP_LOG_BUDGET_MB` | unset (unlimited) | Fleet default per app, in MiB. 0 or unset means no limit |

A per-app value of 0, or `clear`, falls back to the fleet default. Changing a budget needs the same ability as enabling tracing.

## How trimming works

The hourly log retention sweep checks each app against its budget. An app over budget loses its oldest lines, in batches, until it is under. Each app drains a bounded amount per sweep (20,000 lines), so a very large backlog is worked down over several sweeps instead of blocking the database. Search stays consistent because trimmed lines leave the full text index at the same time.

The running total of trimmed lines per app is kept and shown as **Trimmed** in the table.

## Trim now

An app over its budget waits for the hourly sweep. To free the space immediately:

```bash
levelrail-cli apps log-budget trim web
```

In the dashboard, an app over budget shows a **Trim now** button on its Logs page and in **Settings, Observability, Log volume**. It drops the oldest lines of that app only, up to the same per-pass bound as the sweep (20,000 lines), and says when a backlog needs another pass. API: `POST /api/v1/apps/{name}/log-budget/trim` (same ability as changing the budget).

## See the noisiest apps

```bash
levelrail-cli logs usage
```

The table lists apps noisiest first with stored size, line count, ingest rate (bytes per minute averaged over the last 24 hours), budget, percent used, forecast and trimmed lines. The forecast is days until the budget is reached at the current rate. It is empty when the app has no budget or is idle, and "Reached" when it is already at the limit.

The same data is available at `GET /api/v1/logs/usage`, limited to apps the caller can read.

## Cost of the numbers

Usage is a pair of grouped aggregates over the log table (all rows, and the last 24 hours), computed once and cached for 30 seconds. Sizes count the message plus structured fields, not index overhead, so the on-disk footprint is somewhat larger than the budget.
