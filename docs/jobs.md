---
title: "Jobs"
description: Run a command once, to completion, with N completions, a parallelism cap, retries with exponential backoff and a final Succeeded or Failed status.
"og:url": "https://docs.levelrail.com/jobs"
---

For commands that run on a schedule, see [Scheduled jobs](/scheduled-jobs). This page covers the other kind: a command you run once and want to know finished.

## One-shot jobs

A one-shot job runs a command in a fresh container built from an app's current image, with the app's env, secrets, volumes and network, the same way a scheduled job does in its default mode. It can run the command several times, some of them side by side, retry failures, and report one final status.

```bash
levelrail-cli jobs run web --command "node scripts/backfill.js" \
  --completions 10 --parallelism 3 --backoff-limit 4 --timeout 15m --wait
```

| Flag | Meaning |
| --- | --- |
| `--command` | The command. Quotes group words; no shell is involved, so use `sh -c '...'` for a pipeline. You can also pass the argv after `--`. |
| `--completions N` | Runs that must succeed. Default 1. |
| `--parallelism P` | Most runs in flight at once. Default 1. |
| `--backoff-limit B` | Failed attempts tolerated across the whole job before it is declared Failed. |
| `--timeout D` | Timeout of each attempt. |
| `--wait` | Block until the job settles. The exit code is 5 when it failed. |

A failed attempt is retried after an exponentially growing delay (first delay `APP_JOBS_RETRY_BACKOFF`, capped by `APP_JOBS_RETRY_BACKOFF_MAX`). Every attempt is a normal run: it appears in `jobs history <app>` with its output and exit code. An attempt that has to wait because the app already has `APP_JOBS_MAX_CONCURRENT_PER_APP` jobs running is not a failure and spends none of the backoff budget.

The job ends `Succeeded` when all completions succeeded, or `Failed` once more than `backoff-limit` attempts have failed. The status carries the counts: completions succeeded, failed attempts, runs in flight and the IDs of the runs.

### API

`POST /api/v1/jobs/runs` starts a job and answers 202 with its status. `GET /api/v1/jobs/runs/{id}` returns the status. Starting needs the same permission as running a scheduled job now (deploy on the app); reading needs read.

```json
{ "app": "web", "command": ["node", "scripts/backfill.js"], "completions": 10, "parallelism": 3, "backoff_limit": 4, "timeout_seconds": 900 }
```

### Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_JOB_DEFAULT_BACKOFF_LIMIT` | `3` | Backoff limit when the request gives none. `0` means no retries. |
| `APP_JOB_DEFAULT_TIMEOUT` | `10m` | Per-attempt timeout when the request gives none. |
| `APP_JOB_MAX_COMPLETIONS` | `100` | Largest `completions` accepted. |
| `APP_JOB_MAX_PARALLELISM` | `10` | Largest `parallelism` accepted. |
| `APP_JOB_ONESHOT_KEEP` | `100` | Finished jobs whose status is kept in memory. |

### Limits today

- The job summary is held in memory by the control plane. After a restart `GET` answers 404, though the individual runs stay in the run history. A run in flight during a restart is marked interrupted and not resumed.
- The command runs against an app. Running a bare image with no app is not supported yet.
- Each job leaves a hidden, disabled task row so its runs have a history to live in. It is named `oneshot-<job id>` and shows in `jobs list`.
