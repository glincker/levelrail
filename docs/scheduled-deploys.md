---
title: "Scheduled deploys"
description: Redeploy the latest commit on a branch on a recurring cron schedule, or schedule a one-shot image or git ref deploy, with freeze-window and restart-safe catch-up semantics.
"og:url": "https://docs.levelrail.com/scheduled-deploys"
---

A scheduled deploy redeploys the latest commit on a branch on a cron
schedule, with no git push or manual trigger. Use it for a nightly rebuild
against a fresh base image, or a staging environment that tracks `main`
once a day.

You need a connected git source (`PUT /api/v1/apps/{name}/git-source`).
A schedule redeploys through the same path a webhook push does, so there
has to be a repo to redeploy from.

## Configure

<Tabs>
<Tab title="UI">

Open the app, then **Deploy settings**, and use the **Scheduled deploys** card. It has the same fields as the CLI, a next-run preview, and a short history list.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps schedule set web --cron "0 3 * * *" --branch main --timezone UTC
levelrail-cli apps schedule get web
levelrail-cli apps schedule history web
```

</Tab>
<Tab title="API">

`PUT /api/v1/apps/{name}/schedule` with `{cron, branch, timezone?, enabled?}`. The full endpoint list is under [API](#api).

</Tab>
</Tabs>

The cron expression is standard 5-field syntax (minute hour
day-of-month month day-of-week) and is validated on save; an invalid
expression or an unrecognized timezone is rejected with a clear error,
nothing is written. `timezone` defaults to `UTC`. `enabled` defaults to
`true`; set it to `false` (CLI: `--disable`) to keep a schedule configured but paused.

## How it fires

A scheduler inside the control plane process checks every
`APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL` (default `1m`; standard cron has
no granularity finer than a minute, so a shorter interval buys nothing)
which schedules are due, and redeploys the latest commit on the
configured branch through the same git-source deploy path a webhook push
uses. The resulting deploy is recorded with trigger `schedule`
(`GET /api/v1/deployments?trigger=schedule`, or `levelrail-cli apps deploys
list`), distinct from `git push`, `manual`, `api`, `rollback` and
`preview`.

### Freeze windows

A schedule due while a [deploy freeze window](/deploy-safety#freeze-windows)
is active is skipped, not forced: no deploy is triggered, and the
skip is recorded in the schedule's history with the freeze's reason and
end time. It is not queued to run once the freeze lifts; it simply waits
for its next natural occurrence, the same way a schedule due outside any
freeze always has.

### Restart and catch-up semantics

Each schedule persists its own next-fire time in the database, not only
in the control plane process's memory. Two cases follow from that:

- A schedule seen for the first time (just created, or just
  re-enabled) is armed to its next occurrence and does not fire
  immediately. This matches Levelrail's other cron-driven features
  (scheduled tasks, scheduled backups).
- When the control plane restarts, a schedule that was due, or that
  missed one or more occurrences while the process was down, still fires
  at most once the next time it is checked, for the most recent due
  occurrence, never once per missed occurrence. It then resumes its
  normal cadence. This is a deliberate difference from scheduled tasks
  and scheduled backups, whose position lives only in memory and is
  simply lost on restart: a missed scheduled deploy is worth catching up
  once, a missed backup or task run is not.

The next-fire time is re-armed *before* the redeploy is actually
triggered. If the process is killed in the narrow window between arming
and the redeploy call returning, that one occurrence is not retried on
the next tick, and it is never fired twice either: never doubling a
deploy takes priority over never missing one.

## One-time deploy

Instead of `cron`, a schedule can carry `run_at`: an RFC 3339 timestamp at
which the deploy fires once. Set exactly one of `cron` or `run_at`; both or
neither is rejected. `run_at` must be at least
`APP_SCHEDULE_RUN_AT_MIN_LEAD` (default `1m`) and at most
`APP_SCHEDULE_RUN_AT_MAX_HORIZON` (default `8760h`, one year) from now.

```bash
levelrail-cli apps schedule set web --run-at 2026-12-01T03:00:00Z --branch main
levelrail-cli apps schedule set web --run-at 2h --branch main   # converted to an absolute time and printed back
curl -X PUT -H "Authorization: Bearer $TOKEN" -d '{"run_at":"2026-12-01T03:00:00Z","branch":"main"}' \
  https://levelrail.example.com/api/v1/apps/web/schedule
```

The scheduler fires it at the first tick at or after `run_at`, through the
same deploy path as a recurring schedule, then marks it completed and
disabled so it never fires again. `GET .../schedule` shows `run_at`,
`next_run_at` (equal to `run_at` while pending) and `completed_at` once done;
history records the run like any other.

- **Freeze windows:** a one-time deploy due during a freeze is recorded as
  `skipped_freeze` and completed. It is not queued until the freeze lifts.
- **Catch-up:** if the control plane was down at `run_at`, the deploy fires
  once on restart when it is within `APP_SCHEDULE_ONESHOT_CATCHUP` of
  `run_at` (default `24h`, `0` for no limit). Later than that it is recorded
  as `skipped_missed` and completed.
- **Re-arm:** a `PUT` with a new `run_at` on a completed schedule re-arms it.
- **Cancel:** disable it with `apps schedule set ... --disable` (or
  `"enabled": false`) before it fires. There is no separate delete command.

## One-shot scheduled deploys

A one-shot scheduled deploy has its own id, so an app can have several, each
listed and canceled on its own. It deploys either an **image** or the latest
commit of a git **ref** (a branch) once at a set time. This is separate from
the single per-app schedule above, which stays the way to run a recurring
cron deploy.

<Tabs>
<Tab title="UI">

In **Deploy**, choose an image, tick **Schedule for later**, and pick a date and
time (shown in your browser timezone). The **Scheduled deploys** card on the
app's **Deploys** tab lists pending and recent ones and cancels a pending one.
The run itself appears in deploy history with the trigger "a schedule".

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps deploy web --image registry.example.com/web:v2 --at "in 2h"
levelrail-cli apps deploy web --latest --branch release --at 2026-12-01T03:00:00Z --json
levelrail-cli apps schedule list web --status pending
levelrail-cli apps schedule cancel web sdep_0123456789abcdef
```

`--at` takes an RFC 3339 time or a duration such as `in 2h`. The same
`Idempotency-Key` handling as a normal deploy applies, so a retried CI step
creates one schedule.

</Tab>
<Tab title="API">

`POST /api/v1/apps/{name}/scheduled-deploys` with `{run_at, image}` or
`{run_at, ref}` (exactly one of `image` or `ref`; `ref` needs a connected git
source). `GET` lists (`?status=pending,fired`), `GET .../{id}` reads one, and
`DELETE .../{id}` cancels a pending one (409 once it has started).

</Tab>
</Tabs>

`run_at` follows the same minimum lead and horizon as a one-time schedule
(`APP_SCHEDULE_RUN_AT_MIN_LEAD`, `APP_SCHEDULE_RUN_AT_MAX_HORIZON`). Creating
and canceling need the `deploy` ability, reading needs `read`.

### How it runs

A reconciler scans every `APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL` for pending
rows whose `run_at` has passed, so it is level-triggered and survives a restart.

- **Grace:** a row still within `APP_SCHEDULED_DEPLOY_GRACE` (default `5m`) of
  `run_at` runs. An older one is marked `missed`, recorded on the app timeline
  and sent to the app's deploy notification targets.
- **Freezes:** a deploy due inside a [freeze window](/deploy-safety#freeze-windows)
  is marked `skipped_freeze` with the freeze reason and end time. It is not
  queued until the freeze lifts.
- **Gates:** it goes through the normal deploy path: deploy policy, image
  signature enforcement and the running-deploy guard. For an app in a protected
  environment an image deploy becomes a pending [deploy approval](/deploy-policies)
  attributed to whoever scheduled it (status `pending_approval`); a git ref deploy
  is refused there, so use an image.
- **Never twice:** a row is claimed before it executes. One left in `running`
  longer than `APP_SCHEDULED_DEPLOY_STALE_RUN` (default `30m`), for example after
  a crash, is closed as `failed` and not retried.
- **History:** the attempt is recorded with trigger `schedule`, the same as a
  recurring scheduled deploy.

Promotions between environments cannot be scheduled yet.

## API

| Method | Path | Notes |
| --- | --- | --- |
| `GET` | `/api/v1/apps/{name}/schedule` | 404 when no schedule is configured. |
| `PUT` | `/api/v1/apps/{name}/schedule` | `{cron or run_at, branch, timezone?, enabled?}`. Requires a connected git source. |
| `GET` | `/api/v1/apps/{name}/schedule/history` | Newest first; `?limit=` (default 20, max 100). |

## MCP

`get_app_schedule` and `get_app_schedule_history` are read-only. Changing
a schedule stays with the dashboard and CLI, the same split deploy
freeze windows already use.

## See also

- [Deploy safety](/deploy-safety) for freeze windows and the
  stale-deploy guard a scheduled deploy respects the same way any other
  automatic deploy does.
- [Git integrations](/git-integrations) for connecting the git source
  a schedule redeploys from.
