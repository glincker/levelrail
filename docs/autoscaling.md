---
title: "Autoscaling"
tag: "Beta"
description: "Scale an app between a minimum and a maximum replica count on CPU, memory or request load, with cooldowns and a recorded reason for each decision."
"og:url": "https://docs.levelrail.com/autoscaling"
---

Autoscaling keeps an app's replica count in step with its load. You give an app a minimum, a maximum, a metric and a target. Every 30 seconds the control plane compares the metric with the target and, when it is clearly off, writes a new desired replica count. The normal reconciler then starts or stops containers to match.

The decision is plain arithmetic on measured numbers. The same inputs always give the same answer. No model, forecast or AI is involved.

## Set a policy

<Tabs>
<Tab title="Dashboard">

Open the app, choose **Deploys**, and find the **Autoscaling** card. Set a minimum, a maximum, the metric and the target, then save. The card shows the replica count now and the count the autoscaler wants, the latest metric reading, the reason for the last decision, and a timeline of recent decisions.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps autoscale set web --min 2 --max 8 --metric cpu_percent --target 60
levelrail-cli apps autoscale show web
levelrail-cli apps autoscale history web
levelrail-cli apps autoscale clear web
```

A flag on `set` changes only its own field, so `apps autoscale set web --target 70` leaves everything else as it was.

</Tab>
<Tab title="app.yaml">

```yaml
version: 1
services:
  web:
    build: { type: dockerfile }
    port: 3000
    domains: [app.example.com]
    resources: { cpu: 0.5, memory: 512Mi }
    autoscale:
      min: 2
      max: 8
      metric: cpu_percent
      target: 60
```

Leave `replicas` out. See the [`Autoscale` fields](/app-spec-reference#autoscale).

</Tab>
</Tabs>

## Metrics

| Metric | Value compared with `target` |
| --- | --- |
| `cpu_percent` | Average CPU over the last 2 minutes, as a percent of the app's CPU limit (`resources.cpu`). With no limit it is percent of one core. |
| `memory_percent` | Average memory use over the last 2 minutes, as a percent of the app's memory limit. Unknown when the app has no limit. |
| `requests_per_replica` | Requests per second measured at the ingress, divided by the current replica count. See [Traffic observability](/traffic-observability). |

CPU and memory are sampled from one container per collection tick, so they read the load of a typical replica, which is what a balanced app needs.

## How a decision is made

For a reading outside the dead band, the wanted count is `ceil(current * value / target)`, the same proportional rule Kubernetes uses.

- **Dead band.** Nothing moves while the reading is within `scale_up_tolerance` above the target (default 10 percent) and `scale_down_tolerance` below it (default 30 percent). The band is wide below on purpose: removing capacity should need clear evidence.
- **Scale up is fast.** A scale up waits only for `scale_up_cooldown` (default 1 minute) since the last scale up, and adds at most `max_step_up` replicas (default 4) at a time.
- **Scale down is slow.** A lower count must be recommended continuously for `stabilization` (default 5 minutes), and the app lands on the highest count recommended during that window. It also waits `scale_down_cooldown` (default 10 minutes) after any change, and removes at most `max_step_down` replicas (default 1) at a time.
- **No oscillation by construction.** A scale down never goes below the count that would put the metric back at the target, so the new reading cannot cross the scale up threshold on its own. Tolerances, cooldowns and steps cannot be switched off: an unset value takes its default.
- **Bounds always hold.** If the running count is outside `min` and `max` (for example after you lower `max`), it is corrected at once, without waiting for a cooldown.

## When it holds, and why

Every tick records one of these reasons. The dashboard, `apps autoscale show` and the history all use them.

| Reason | Meaning |
| --- | --- |
| `WithinTarget` | The reading is inside the dead band. |
| `ScaledUp`, `ScaledDown` | The count changed. |
| `CooldownActive` | A change is wanted but the cooldown has not passed. |
| `Stabilizing` | A scale down is being confirmed over the stabilization window. |
| `MetricsUnavailable` | The metric is missing or stale. The count is held, never changed on unknown data. |
| `SpendCapPaused` | A [spend cap](/spend-caps) is in force. A budget that is throttled or paused wins over scale up, so no replicas are added. Scale down still works. |
| `NoCapacity` | Placement says the app's node cannot fit another replica. The count is held and the condition shows it. |
| `AtMaximum`, `AtMinimum` | The limit is reached. `AtMaximum` is flagged as needing attention because load wants more. |
| `BoundsCorrected` | The count was outside `min` and `max` and was moved back inside. |
| `Suspended` | The app is stopped or asleep, so its replicas are left alone. |
| `ScaleFailed` | Writing the new count failed. The cooldown is given back so the next tick retries. |

A decision that was cut short also records what limited it in `blocked_by`: `cooldown`, `stabilization`, `spend_cap`, `no_capacity`, `max_replicas`, `global_ceiling`, `step_limit` or `no_metrics`.

Silence is not zero. For `requests_per_replica`, a window with no requests counts as idle only while the container's own samples are fresh; otherwise the reading is unknown and the count is held.

## Manual replicas while a policy is set

The autoscaler owns the replica count. A manual edit that would change it (dashboard, `apps scale`, `PUT /api/v1/apps/{name}`) is refused with `409`, and a deploy whose `app.yaml` sets `replicas` fails with the same explanation. To set replicas by hand, clear the policy first: `apps autoscale clear` leaves the count where it is. A deploy never resets the count the autoscaler chose.

## Limits you control

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_AUTOSCALE_MAX_REPLICAS` | `20` | Global ceiling. A policy cannot ask for more, and a lower ceiling set later is honored at once. |
| `APP_AUTOSCALE_INTERVAL` | `30s` | How often every policy is evaluated. |
| `APP_AUTOSCALE_WINDOW` | `2m` | Trailing window metrics are averaged over. |
| `APP_AUTOSCALE_METRIC_MAX_AGE` | `90s` | Oldest the newest sample may be before the metric counts as unknown. |
| `APP_AUTOSCALE_FLAP_THRESHOLD` | `3` | Direction reversals in an hour that count as flapping. |

## Alerts

The autoscaler records four gauges per app each tick, so an ordinary [threshold alert rule](/observability) can watch them:

| Metric | Value |
| --- | --- |
| `autoscale_no_capacity` | `1` while the app wants more replicas and no node can fit them |
| `autoscale_flaps_1h` | Direction reversals in the last hour |
| `autoscale_replicas_current`, `autoscale_replicas_desired` | The count now and the count wanted |

For example, alert when `autoscale_no_capacity` is above 0 for 5 minutes, or when `autoscale_flaps_1h` reaches 3:

```bash
levelrail-cli apps alerts create web --name "autoscale stuck" --kind threshold --metric autoscale_no_capacity --comparator '>' --threshold 0 --for-duration 5m
```

The status condition also reads `Flapping` once the threshold is reached, and the dashboard card says so.

## API

| Route | Needs | Purpose |
| --- | --- | --- |
| `GET /api/v1/apps/{name}/autoscale` | read | Policy, replicas now and wanted, last decision. `404` when no policy is set. |
| `PUT /api/v1/apps/{name}/autoscale` | deploy | Create or replace the policy, then evaluate once. |
| `DELETE /api/v1/apps/{name}/autoscale` | deploy | Remove the policy. Replicas stay as they are. |
| `GET /api/v1/apps/{name}/autoscale/history` | read | Decisions, newest first. |

Every change is audited, and the history records who changed the policy.

## Design notes

- **A controller that writes desired state.** It never touches containers. It writes `replicas` on the app, and the existing reconciler converges. A crash between the two is safe: intent and cooldown are saved first, and a failed write gives the cooldown back.
- **Level-triggered.** Each tick re-derives the decision from stored state and current signals. There is no event queue to lose.
- **Hold on the unknown.** Missing data, an unreadable spend cap state and a failed capacity check all hold the count rather than guess. Capacity that cannot be told at all (single node with no reported capacity) does not block.
- **Why proportional with a dead band, not a PID loop.** It is predictable, explainable in one line per decision, and replayable from a recorded metric trace, which the decision tests do.
- **Not in scope.** Predictive or learned scaling, scaling to zero (see [sleep when idle](/sleep-when-idle)), custom metrics, and scheduling replicas across several nodes.
