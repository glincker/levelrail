# Scheduled job runner

Named jobs that a cron trigger calls. Each run is recorded in the platform key-value store, and a short-lived lock stops two runs of one job overlapping. Add your own jobs to the `jobs` object in `handler.js`; two examples ship (`heartbeat`, `prune-runs`).

## Environment

| Variable | Required | Meaning |
| --- | --- | --- |
| `KV_URL`, `KV_TOKEN` | yes | Injected when you enable the key-value store for the function. |
| `CRON_SECRET` | yes | Secret path segment. The scheduler sends no custom headers, so the job path is `/cron/<CRON_SECRET>/<job>`. Every call is refused while it is unset. |
| `HEARTBEAT_URL` | no | Address the `heartbeat` job pings. |
| `JOB_LOCK_SECONDS` | no | Lock lifetime, so a crashed run cannot block forever. Default 300. |
| `RUN_HISTORY_TTL_SECONDS` | no | How long run records are kept. Default 604800. |

## Schedule it

Create an http job on the function's app. It wakes the function if it is asleep:

```
levelrail-cli jobs create <app> heartbeat --mode http \
  --http-path /cron/<CRON_SECRET>/heartbeat --schedule "*/15 * * * *"
```

`GET /runs` shows the last 20 runs. See the scheduled functions guide for retries and overlap control on the platform side.

## Test

```
node --test
```
