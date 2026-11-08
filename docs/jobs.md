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

`jobs run` is the way to run a one-off command today, for example a migration or a console task. The shorter form below is the usual way.

A failed attempt is retried after an exponentially growing delay (first delay `APP_JOBS_RETRY_BACKOFF`, capped by `APP_JOBS_RETRY_BACKOFF_MAX`). Every attempt is a normal run: it appears in `jobs history <app>` with its output and exit code. An attempt that has to wait because the app already has `APP_JOBS_MAX_CONCURRENT_PER_APP` jobs running is not a failure and spends none of the backoff budget.

The job ends `Succeeded` when all completions succeeded, or `Failed` once more than `backoff-limit` attempts have failed. The status carries the counts: completions succeeded, failed attempts, runs in flight and the IDs of the runs.

### Run a command with `apps run`

```bash
levelrail-cli apps run web -- bin/rails db:migrate
levelrail-cli apps run shop --service worker --timeout 5m -- node scripts/backfill.js
```

`apps run` starts a one-shot job, waits for it, prints the output and exits with the command's own exit code, so it works in scripts and CI. No terminal is needed. The output is printed when the command finishes, not line by line. A timeout exits 124, and a job that failed without a command exit code (the container could not start) exits 5. Secret values are redacted from the output, and the command line is never echoed. `--service S` targets one service of a multi-service app. `--json` prints the finished job, output included, and nothing else. `apps run` sends a backoff limit of 0, so a failed command runs once; `--retries N` allows N more attempts. Use `apps exec` instead to run inside the live container.

The dashboard has the same thing on an app's **Exec** tab, in the **Run in a fresh container** card. It splits the line into arguments without a shell, always sends a backoff limit of 0, polls the job once a second and shows the output and exit code when it ends. The Exec tab's first card, by contrast, runs inside the live container.

### API

`POST /api/v1/jobs/runs` starts a job and answers 202 with its status. `GET /api/v1/jobs/runs/{id}` returns the status. One-off runs and scheduled jobs follow console exec access rules: starting a one-off run, and creating, updating or running a scheduled job now, needs the root ability on the app and exec enabled for that app (`PUT /api/v1/apps/{name}/exec-access`). Reading needs read. The `GET` answer also carries the newest attempt's `exit_code`, `run_status` and `output` (at most `APP_JOBS_MAX_OUTPUT_BYTES` bytes, default 65536, the same limit a run keeps; `output_truncated` is set when the output was cut), so one call is enough to see what the command printed. The `output` field (and a job's `last_run_output`) is only returned to callers with the read_sensitive ability, since a command can print values that redaction does not know about; a read-only caller gets the status and exit code plus `output_withheld: true`.

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
| `APP_JOB_ONESHOT_KEEP` | `100` | Finished jobs whose status is kept. Older ones are pruned together with their hidden task row. |

### Limits today

- Job status is stored, so `GET` keeps answering after a restart. A job that was still running when the control plane restarted is marked `Failed` with a message saying so, and its progress counts are kept. It is not resumed, because that could run the command twice: start a new job to redo the remaining work.
- The command runs against an app. Running a bare image with no app is not supported yet.
- Each job leaves a hidden, disabled task row so its runs have a history to live in. It is named `oneshot-<job id>`, shows in `jobs list` and is removed when the job is pruned.
