---
title: "Scheduled jobs"
description: Run commands on a cron schedule against your app, in a fresh container with the app's env, secrets and volumes, with run history, logs, retries and alerts.
"og:url": "https://docs.levelrail.com/scheduled-jobs"
---

A job runs a command on a cron schedule for one app: a database migration, a
cache warm-up, a nightly report. Every run is recorded with its start, end,
exit code, duration, trigger and node, and its output is kept so you can read it
later from the dashboard or the CLI.

Jobs live on the app's **Jobs** tab, under `levelrail-cli jobs`, in
`app.yaml`, and at `/api/v1/apps/{name}/jobs`. They are the same jobs on every
surface.

## Where a job runs

| Mode | What happens |
| --- | --- |
| `fresh` (default) | A new container starts from the app's current image with the app's env vars, secrets, volumes and network, runs the command, and is removed. Like `docker run --rm` with the service's own spec. |
| `exec` | The command runs inside the app's running container. It fails with "container not running" when the app is down. |

A fresh job gets no published ports and no restart policy, and only the volumes
the app already declares. It runs as the image's user under the same Docker
guard rules as the app itself. On a [multi-node](/multi-node) setup the job runs on the node
the app is placed on.

The command is a real argv list with no shell involved. Use
`["sh", "-c", "..."]` for pipelines. The dashboard form wraps what you type in
`sh -c` for you. For a fresh job the command replaces the image's entrypoint, so
what you write is exactly what runs.

## Create a job

### Dashboard

Open the app, choose **Jobs**, then **Create job**. Pick the schedule
(daily, weekly or a cron expression), the time zone, and the policies below.
Use **Run now** on a row to try it immediately.

### CLI

```bash
levelrail-cli jobs create web nightly-migrate \
  --schedule "0 3 * * *" --timezone Europe/Berlin \
  --retries 2 --timeout 10m --concurrency forbid \
  -- sh -c "bin/migrate up"

levelrail-cli jobs list web
levelrail-cli jobs run web nightly-migrate
levelrail-cli jobs history web --status failed
levelrail-cli jobs logs web nightly-migrate
levelrail-cli jobs disable web nightly-migrate
```

Jobs belong to one service. If the name you pass is an app group (a multi-service app), the 404 lists the group's services; run the job on one of them, for example `jobs run my-app-web nightly-migrate`.

`<job>` is the job's name or its ID. `jobs logs` prints the latest run's output,
or a specific one with `--run RUN_ID`.

### app.yaml

```yaml
version: 1
services:
  web:
    build:
      type: dockerfile
      path: ./Dockerfile
    port: 3000
    jobs:
      - name: migrate
        schedule: "0 3 * * *"
        command: ["sh", "-c", "bin/migrate up"]
        timezone: Europe/Berlin
        concurrency: forbid
        timeout: 10m
        retries: 2
        retryBackoff: 30s
      - name: warm-cache
        schedule: "*/15 * * * *"
        command: ["bin/warm-cache"]
        memory: 256Mi
        cpu: 0.5
      - name: weekly-report
        schedule: "0 8 * * 1"
        command: ["bin/send-report", "--to", "team@example.com"]
        warnAfter: 30m
        catchUp: run-once
        catchUpLookback: 12h
```

Every deploy makes the service's `app.yaml` jobs match the list: new ones are
created, changed ones updated in place (keeping their history), and ones you
remove are deleted. A spec job is marked `app.yaml` in the dashboard and can only
be enabled or disabled there. Jobs you create in the dashboard or CLI are never
touched. A spec with no `jobs:` key leaves existing jobs alone; `jobs: []`
removes every spec job.

| Field | Default | Meaning |
| --- | --- | --- |
| `name` | required | Unique per app, lowercase letters, digits, `-`, `_`. |
| `schedule` | required | 5-field cron expression. |
| `command` | required | Argv list. |
| `timezone` | `UTC` | IANA zone the schedule is read in. |
| `mode` | `fresh` | `fresh` or `exec`. |
| `concurrency` | `forbid` | `allow`, `forbid` (skip while one is running) or `replace` (cancel the old run). |
| `timeout` | 10m | Per attempt. |
| `retries` | 0 | Extra attempts after a failure. |
| `retryBackoff` | 10s | First retry delay, doubled each retry, capped at 5m. |
| `catchUp` | `skip` | `skip` or `run-once` after downtime. |
| `catchUpLookback` | 1h | How far back `run-once` still applies. |
| `memory`, `cpu` | the app's | Limits for a fresh container. |
| `warnAfter` | none | Raises the running-too-long alert. |
| `enabled` | `true` | Set `false` to keep the job but not run it. |

Jobs created through the API default to `concurrency: allow`.

## When jobs run, and what survives a restart

The scheduler is level-triggered: each tick it works out which cron occurrence
is due from the job's saved high-water mark and the clock, claims that
occurrence in the database, and only then runs it. Because the claim is saved
before the command starts:

- A control plane restart never runs the same occurrence twice.
- A run that was in flight when the control plane stopped is marked
  **interrupted** and its container is removed. It is not retried, since the
  command may already have done real work.
- If the control plane was down across an occurrence, the job is either run once
  (`catchUp: run-once`, when the occurrence is inside the lookback window) or
  recorded as **missed** (`skip`). Nothing is dropped silently, and a job that
  was only just enabled does not backfill.

Time zones follow daylight saving: a wall time that does not exist on a spring
forward day runs once at the shifted time, and a repeated wall time on a fall
back day runs once.

## History and logs

The Jobs tab shows the run history for the app, newest first, with a status
filter and a log view per run. The same data is at
`GET /api/v1/apps/{name}/job-runs` and `GET .../jobs/{job}/runs/{run}`. Each run
records the trigger (`schedule`, `manual` or `api`), attempts, exit code,
duration and node.

Output keeps the last 64 KiB of stdout and stderr. Values of the app's secrets
(and any env var whose name looks like a password, token, key or DSN) are
replaced with `[redacted]` before the output is stored.

Output of a fresh job is captured on the control plane's own node. On a remote
node the run still records its exit code and timing, but not output; use
`exec` mode when you need the output from a remote node.

## Alerts

Create alert rules on a job from **Alerts** or `levelrail-cli apps alerts create`
with `--scheduled-task-id <job id>`:

| Kind | Fires when |
| --- | --- |
| `scheduled_task_failure` | The job failed (after its retries) the given number of runs in a row. |
| `scheduled_task_missed` | Scheduled runs were missed (the control plane was down and catch-up did not cover them). |
| `scheduled_task_running_long` | A run has been going longer than the job's `warnAfter`. |

They use the same notification channels as every other alert.

## Limits and settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_JOBS_MAX_PER_APP` | 20 | Jobs per app. |
| `APP_JOBS_MAX_CONCURRENT_PER_APP` | 3 | Runs of one app in flight at once; more are skipped. |
| `APP_JOBS_DEFAULT_TIMEOUT` | 10m | Timeout for jobs that set none. |
| `APP_JOBS_MAX_RETRIES` | 5 | Upper bound on a job's retries. |
| `APP_JOBS_RETRY_BACKOFF` | 10s | Default first retry delay. |
| `APP_JOBS_RETRY_BACKOFF_MAX` | 5m | Cap on the doubling backoff. |
| `APP_JOBS_CATCH_UP_LOOKBACK` | 1h | Default catch-up window. |
| `APP_JOBS_HISTORY_KEEP` | 200 | Runs kept per job. |
| `APP_JOBS_OUTPUT_RETENTION` | 720h | How long a run's output is kept. |
| `APP_JOBS_MAX_OUTPUT_BYTES` | 65536 | Output kept per run. |
| `APP_JOBS_BACKGROUND_TIMEOUT` | 15m | How long a background [http job](/scheduled-functions) is awaited. |
| `APP_SCHEDULED_TASK_SCHEDULER_INTERVAL` | 1m | How often the scheduler looks for due jobs. |

## Examples

Run migrations nightly and alert if they fail twice in a row:

```yaml
jobs:
  - name: migrate
    schedule: "0 3 * * *"
    command: ["sh", "-c", "bin/migrate up"]
    retries: 1
    concurrency: forbid
```

Warm a cache every 15 minutes with a small memory cap:

```bash
levelrail-cli jobs create web warm-cache --schedule "*/15 * * * *" \
  --memory 256 --cpu 0.5 -- bin/warm-cache
```

Email a report every Monday morning, and catch up if the server was rebooting:

```bash
levelrail-cli jobs create web weekly-report --schedule "0 8 * * 1" \
  --timezone America/New_York --catch-up run_once --catch-up-lookback 6h \
  --warn-after 30m -- sh -c 'bin/report | mail -s "Weekly report" team@example.com'
```

## Related

- [app.yaml reference](/app-spec-reference)
- [Observability and alerts](/observability)
- [Scheduled deploys](/scheduled-deploys)
