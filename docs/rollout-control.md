---
title: "Rollout control"
description: "Tune rolling deploys with max_surge and max_unavailable, and protect running replicas with a min_available disruption budget."
"og:url": "https://docs.levelrail.com/rollout-control"
---

With `strategy: rolling` and more than one replica, Levelrail replaces
replicas in waves. Two settings decide how big a wave is, and a third
protects your app from operations that would take too many replicas down
at once.

```yaml
services:
  web:
    replicas: 6
    strategy: rolling
    rollout:
      max_surge: 2          # up to 2 extra replicas while a wave starts
      max_unavailable: 25%  # up to 1 of 6 replicas may be down during the rollout
      min_available: 4      # voluntary operations must leave 4 replicas up
```

Each value is a whole number or a percent of `replicas`. Percents round up
for `max_surge` and `min_available`, and down for `max_unavailable`, so
rounding never makes a rollout less safe.

## How a wave runs

A wave replaces `max_surge + max_unavailable` replicas:

1. Up to `max_unavailable` old replicas stop first.
2. The wave's new replicas start one after another, and each has to pass
   its readiness check.
3. The rest of the wave's old replicas stop.

Healthy replicas never drop below `replicas - max_unavailable`. Extra
containers stay within `max_surge`, except while a drain window or a held
previous release keeps old replicas running on purpose (see
[Deploy safety](/deploy-safety)).

If a new replica fails readiness or crash loops, the rollout stops at that
wave. The usual crash loop rollback then applies, the same as for a single
replica deploy.

## Pause and resume

A rolling deploy can be held between waves so you can inspect the new
replicas before the rest start:

```bash
levelrail-cli rollout pause web
levelrail-cli rollout resume web
levelrail-cli rollout abort web
```

A pause takes effect at the next wave boundary, so a wave that already
started finishes first. While paused, `rollout waves` and the dashboard
show the rollout as paused at that wave, and a restart of the control
plane keeps the pause. `rollout abort` rolls the app back to its previous
deploy, the same as `apps rollback`, and the hold lapses because the
release changed. The dashboard shows Pause and Resume on the rollout panel
of a deploy. The API is `POST /api/v1/apps/{name}/rollout/pause` and
`/resume`, both needing the deploy ability.

## Defaults

Unset values come from runtime settings, which you change live in Settings
(Deploys), with `levelrail-cli settings set`, or through the API. The next
rollout uses the new value.

| Setting | Meaning | Seeded from |
| --- | --- | --- |
| `rollout.max_surge` | default `max_surge` | `APP_ROLLOUT_MAX_SURGE` |
| `rollout.max_unavailable` | default `max_unavailable` | `APP_ROLLOUT_MAX_UNAVAILABLE` |
| `rollout.min_available` | default `min_available` | `APP_ROLLOUT_MIN_AVAILABLE` |

The env variable only seeds a setting that was never saved. With nothing
set, a rollout replaces one replica at a time and keeps every replica
serving, which is how rolling deploys worked before these settings
existed. An invalid value is ignored and that default is used instead.

## Disruption budget

`min_available` blocks voluntary operations that would leave fewer
replicas up than it allows:

- `apps stop` (all replicas stop)
- `apps restart` with `strategy: rolling` when `max_unavailable` would
  breach the budget (blue-green and canary restarts prove the new set
  first, so they are never blocked)
- node drain, for every running app on that node

A blocked call returns HTTP 409, and the message says what to do: scale
the app up first, wait for its replicas to be ready, or lower
`rollout.min_available` in app.yaml. To go ahead anyway, call the API
with `?force=true`.

## Watching a rollout

```bash
levelrail rollout waves web          # policy, wave N of M, replica counts
levelrail rollout waves web --json   # the same, as JSON
levelrail rollout status web         # the counts, then wait for convergence
```

The API is `GET /api/v1/apps/{name}/rollout`. It returns the saved policy,
what that policy resolves to for the current replica count, and the last
recorded progress: wave, waves, updated, ready, unavailable and surge.
Progress is kept in memory, so it is empty again after the control plane
restarts. In the dashboard, the Rollout tab of a deploy shows the same
counts while a rolling deploy runs.

## The held previous release

After a cutover the previous release keeps running for the hold window so a
rollback is instant. Docker cannot relabel a running container, so the new
release's containers carry `platform-reserved.supersedes=<container base name>`
naming the release they replaced; `docker ps --filter
label=platform-reserved.supersedes` shows which older container is that held
release and not a stray duplicate. The app page shows a "Previous release
held, removed in ..." chip until the hold ends (`previous_release_held_until`
in `GET /api/v1/apps/{name}`).

Deleting an app also deletes its saved rollout progress row.
