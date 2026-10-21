---
title: "Scheduled and background functions"
description: Call a path on your app on a cron schedule, wake it if it sleeps, and let long calls finish in the background with retries, overlap control and run history.
"og:url": "https://docs.levelrail.com/scheduled-functions"
---

An **http job** calls a path on your app on a cron schedule instead of running a
command. It is the Levelrail version of a scheduled function: the app keeps its
handler, the control plane keeps the clock. If the app is asleep
([sleep when idle](/sleep-when-idle)), the control plane wakes it, waits until it
accepts connections, then makes the call.

Http jobs are ordinary [scheduled jobs](/scheduled-jobs) with `mode: http`, so
they share the Jobs tab, the CLI, the API, run history, retries, catch-up and
alerts.

## Create one

Dashboard: **App > Jobs > Create job**, set **Where it runs** to *HTTP call to
the app*, then a path, method and schedule. The dialog lists the next five runs
in the job's time zone before you save.

CLI:

```sh
levelrail-cli jobs create web digest --mode http --http-path /cron/digest \
  --schedule "@hourly" --timezone Europe/Berlin --retries 3 --concurrency queue
levelrail-cli jobs next --schedule "0 9 * * 1-5" --timezone America/New_York
```

API: `POST /api/v1/apps/{name}/jobs` with

```json
{
  "name": "digest",
  "mode": "http",
  "schedule": "@hourly",
  "timezone": "Europe/Berlin",
  "concurrency_policy": "queue",
  "http": { "method": "POST", "path": "/cron/digest", "background": false }
}
```

## Schedules

Five fields (`minute hour day-of-month month day-of-week`) or one of `@hourly`,
`@daily`, `@midnight`, `@weekly`, `@monthly`, `@yearly`. The schedule is read on
the wall clock of the job's IANA time zone. A wall time skipped by a daylight
saving jump runs once, an hour later. A wall time repeated by a fall-back runs
once.

## What a call looks like

The control plane sends the method and path to the app's published port, with
`X-Job-Run` (the run ID), `X-Job-Name` and, for background jobs,
`X-Job-Background: 1`. A body, if set, is sent as `application/json`. Any 2xx
answer is a success. Anything else is a failure and is retried with doubling
backoff up to the job's retries. The HTTP status is stored on the run (shown as
*HTTP 200* in history) with the first part of the response body, capped by
`APP_JOBS_MAX_OUTPUT_BYTES`. The path must start with a single `/`; the call
never leaves the app.

## Overlap

| Policy | If the previous run is still going |
| --- | --- |
| `allow` | Start the new run too. |
| `forbid` (skip) | Record the new run as skipped. The default in the dashboard. |
| `replace` | Cancel the old run, then start the new one. |
| `queue` | Wait for the old run to finish, then run, so runs go one after another. |

A schedule is single-flight per occurrence: the control plane persists which
cron occurrence it handled before it runs it, so a restart never runs the same
occurrence twice and never silently loses one. A run that was in flight during a
restart is marked interrupted and is not re-run. Missed occurrences follow the
job's catch-up policy.

## Background functions

Turn on **Background job** (`--background`, `"background": true`) for work that
outlives a normal request. Your handler answers `202` straight away, or just
keeps the connection open and works; either way the control plane, not the
caller, waits for the outcome. It holds the call (and the wake) for up to
`APP_JOBS_BACKGROUND_TIMEOUT` (default 15m) unless the job sets its own timeout,
then records success, failure or timeout in the run history. Triggering a job
with **Run now** (`POST .../jobs/{id}/run`) always returns `202` with a run ID
immediately, whatever the job's duration.

## Pause, resume, run now, history

`levelrail-cli jobs pause|resume <app> <job>` (or the Enabled switch) stops and
restarts the schedule without a backlog: resuming does not replay what was
paused. `jobs run` fires a job now. `jobs history <app> [job]` and the history
table list status, duration, HTTP status, attempts and the response body. The
last `APP_JOBS_HISTORY_KEEP` runs per job are kept.

## Alerts

A failed run raises the existing job alert kinds, so add one rule per job and it
reaches your notification channels like any other alert:

```sh
levelrail-cli apps alerts create web --kind scheduled_task_failure \
  --scheduled-task-id ID --restart-count-threshold 1
```

`scheduled_task_missed` and `scheduled_task_running_long` work the same way.

## Limits

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_JOBS_BACKGROUND_TIMEOUT` | 15m | How long a background http job is awaited. |
| `APP_JOBS_DEFAULT_TIMEOUT` | 10m | Timeout for other jobs that set none. |
| `APP_JOBS_MAX_RETRIES` | 5 | Upper bound on a job's retries. |
| `APP_JOBS_HISTORY_KEEP` | 200 | Runs kept per job. |
| `APP_JOBS_MAX_OUTPUT_BYTES` | 65536 | Response body kept per run. |

The rest are listed with [scheduled jobs](/scheduled-jobs).

## Not covered

Http jobs cannot yet be declared in `app.yaml`, and the call carries no custom
headers. Use the dashboard, CLI or API to manage them.
