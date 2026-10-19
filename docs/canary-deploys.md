---
title: "Canary deploys"
description: Send a share of an app's traffic to a new image in stages, promote it, or let the control plane roll it back when errors or latency rise.
"og:url": "https://docs.levelrail.com/canary-deploys"
---

Run a new image next to the current release and send it a percentage of
requests. If it looks healthy, promote it. If not, abort and every request
goes back to the release that was already serving. Nothing changes for the
stable release until you promote.

## How it works

- Starting a canary creates a hidden clone of the app (`<app>--canary`) with
  the same environment variables and secrets, running the new image. It does
  not appear in the apps list and has no domains of its own.
- Ingress splits each request between the app and the canary by weight (1 to
  99 percent). The split is per request, not per user, so one visitor can see
  both releases. The split follows a repeating cycle of 100 requests, so a
  weight of 20 sends exactly 20 of every 100 requests to the canary. A handful
  of requests may all land on one release.
- A canary that is not running or not ready receives no traffic, so a bad
  image cannot take the app down. Weight 0 pauses the canary without removing it.
- **Promote** deploys the canary image to the app through the normal deploy
  path (readiness gate, rollback target, freeze and protection checks), then
  removes the clone.
- **Abort**, or deleting the app, removes the clone.

## Staged canaries with automatic rollback

Give the canary a stage schedule and the control plane runs it for you. It
sends the first share of traffic, watches the canary against the stable
release, and either moves to the next stage, promotes at the end, or rolls the
canary back.

```
levelrail-cli apps canary start web --image myorg/web:next --stages 10,25,50,100 --interval 5m
```

- Each stage observes the canary's own requests (5xx rate and p95 latency from
  the edge request stats) and the stable release's over the same window.
- The canary is rolled back, and the reason recorded, as soon as it has served
  at least `APP_CANARY_MIN_REQUESTS` requests in the stage and its 5xx rate is
  more than `max_error_rate` above the stable release's, or its p95 latency is
  more than `max_p95_ratio` times the stable release's (and above
  `APP_CANARY_P95_FLOOR_MS`, so tiny latencies do not trip it).
- A stage advances once its interval has passed with enough canary traffic. A
  canary that gets no traffic waits; it is never promoted unobserved.
- A stage of 100 means promote: the image is deployed to the app through the
  normal deploy path and the canary is removed. **Promote now** skips the rest.
- Weight 0 pauses a staged canary. **Abort** removes it.
- The state lives in the database and a reconciler converges on it every
  `APP_CANARY_TICK`, so a control plane restart only reopens the current
  stage's observation window. A failed rollback or promote is retried.

### Defaults and per-app policy

Defaults come from environment variables on the control plane:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CANARY_STAGES` | `10,25,50,100` | Stage schedule. |
| `APP_CANARY_INTERVAL` | `5m` | Observation time per stage. |
| `APP_CANARY_MAX_ERROR_RATE` | `0.02` | Allowed 5xx rate rise over the stable release. |
| `APP_CANARY_MAX_P95_RATIO` | `1.5` | Allowed p95 latency multiple. |
| `APP_CANARY_MIN_REQUESTS` | `30` | Canary requests needed before a stage is judged. |
| `APP_CANARY_P95_FLOOR_MS` | `50` | Latency ratio is ignored below this p95. |
| `APP_CANARY_TICK` | `15s` | How often the reconciler runs. |

An app overrides them in `app.yaml`. A deploy saves the block, and `canary
start` without `--weight` then runs staged with it:

```yaml
services:
  web:
    image: myorg/web:1
    strategy: canary
    canary:
      stages: [10, 25, 50, 100]
      interval: 10m
      max_error_rate: 0.01
      max_p95_ratio: 1.3
```

`strategy: canary` only saves this policy. The stable app still deploys
blue-green, and a canary starts when you start one (CLI, dashboard, API or MCP).

## Limits

- One canary per app.
- Image apps only, with a single replica and no pinned host port. Apps built
  from source, stopped apps, and apps under a deploy freeze are refused.
- The canary has its own container and its own data connections. If the app
  writes to a database, the canary writes to it too.

## CLI

```
levelrail-cli apps canary start web --image myorg/web:next --weight 10
levelrail-cli apps canary weight web --weight 50
levelrail-cli apps canary status web
levelrail-cli apps canary promote web
levelrail-cli apps canary abort web
```

`status` on a staged canary prints the stage, the canary and stable request
counts, error rates and p95, and what happens next. With the canary gone it
prints how the last one ended. All verbs take `--json`.

## Dashboard

App, Deploys: the Canary release card has the image form. Fill in stages for
a staged canary, or a traffic share for a manual one. A running staged canary
shows a stage progress bar, the live error rate and p95 of the canary next to
the stable release, and **Promote now** and **Abort** buttons. After a rollback
the card says why.

## API

`GET`, `POST`, `PUT` and `DELETE /api/v1/apps/{name}/canary`, and
`POST /api/v1/apps/{name}/canary/promote`. All need the `deploy` ability, and
`GET` needs `read`. `POST` takes `stages`, `interval`, `max_error_rate` and
`max_p95_ratio` for a staged canary. `GET` returns the stage, the live
comparison and the last outcome. See the [API reference](/api-reference).

## MCP

`get_canary` reads the same status. `start_canary`, `promote_canary` and
`abort_canary` change state and are registered in `full` mode only.

## Verified

Run on a real control plane with Docker and embedded Caddy: weights of 30 and
20 produced exactly 30 and 20 canary responses per 100 requests, weight 0
sent everything to the stable release, and promote left only the new image
serving. A weight change reaches ingress within seconds. A new canary takes up
to about 30 seconds to start its container.

## Troubleshooting

### Cannot start a canary on this app

**Likely cause:** The app is built from source, has multiple replicas, a pinned host port, is stopped, or is under a deploy freeze.

**How to confirm:** Run `levelrail-cli apps canary start <name> --image <image>`. The error will state which limitation applies.

**Fix:** Canaries work only with prebuilt image apps. If the app is built from source, deploy the new image manually first (push to a registry, then deploy by tag). If it has multiple replicas, scale it to one. If it has a pinned host port, remove it first.

### Canary is not receiving traffic

**Likely cause:** The canary container is not running or failed its readiness check, or the weight is set to 0.

**How to confirm:** Run `levelrail-cli apps canary status <name>` and check the canary app's container status in the dashboard (it is named `<app>--canary` but not listed in the apps view).

**Fix:** Check the canary app's logs with `levelrail-cli apps logs <app>--canary -f` for startup errors. If the image is bad, abort the canary and start a new one with a different image. If the weight is 0, increase it with `levelrail-cli apps canary weight <name> --weight 10`.

### Promote gets stuck or times out

**Likely cause:** The canary image does not pass the readiness gate (same as a normal deploy), or there is a deploy freeze blocking it.

**How to confirm:** Run `levelrail-cli apps deploys show <name>` and check the failure details. Look for a deploy freeze in Settings > Deploy safety.

**Fix:** Check the canary image logs and fix any startup issues. If there is a deploy freeze, promote after the freeze window ends, or override it with a reason.

### Still stuck

For help with a stuck canary deploy, check [Deploy failures](deploy-failures.md) for the failure code, or run `levelrail-cli apps diagnose <name>` and share its output.
