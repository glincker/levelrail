---
title: "Spend caps"
description: "Set a monthly egress and cost limit per app, get alerts at 50, 80 and 100 percent, and choose whether the app is throttled or paused when the limit is reached."
---

A spend cap keeps heavy traffic from turning into a surprise bill. You give an app a monthly egress limit, a monthly cost ceiling, or both. The platform alerts you as usage climbs and, if you choose, throttles or pauses the app when the limit is reached. Counters reset on the first day of each UTC month.

Caps are decided by plain arithmetic on measured usage. No AI is involved in deciding or enforcing anything.

## What is measured

Usage comes from the same daily numbers as [Usage and cost](/usage-and-cost): HTTP response bytes ingress sent for the app, and the cost allocated to the app from the prices you entered.

- **Egress** needs no setup.
- **Cost** needs prices. With no prices configured the cost reads zero, so a cost ceiling never fires. The app's Budget section and `budget show` both say so.
- The binding limit is whichever of the two is closest to exhausted.

## Alerts

By default the app alerts at 50, 80 and 100 percent of the limit. You can pick your own levels (whole percents, up to eight). Each level fires once per month and is sent through the same `budget_threshold` alert rule as project and team budgets, so create that rule once and your existing channels receive app caps too. A newly crossed level notifies again even while the rule is already firing.

## What happens at 100 percent

You pick one action per app:

| Action | Effect |
| --- | --- |
| `none` | Alert only. The app keeps serving. |
| `throttle` | Each client address is limited to a number of requests per second (default 5). |
| `pause` | The app's domains answer with a short "temporarily unavailable" page and a 503 until the month rolls over or you raise the limit. |

Throttling limits request rate per client. It slows heavy callers but is not a bandwidth cap, so a few clients pulling large files can still use a lot of egress. Use `pause` when the limit must hold.

If a domain already has a stricter rate limit, throttle never loosens it. If a domain is already in maintenance mode, your own page is kept.

## How it is enforced

A controller re-evaluates every capped app every two minutes and records a condition with a reason:

| Reason | Meaning |
| --- | --- |
| `WithinBudget` | Under the first alert level. |
| `ThresholdReached` | An alert level was crossed, no action yet. |
| `LimitReachedAlertOnly` | At 100 percent with action `none`. |
| `Throttled` | At 100 percent, the rate limit is applied. |
| `Paused` | At 100 percent, the maintenance response is served. |
| `EnforcementPending` | The decision is saved but ingress has not applied it yet. It retries every tick. |

The decision is recomputed from stored usage each time, so a restart or a missed tick changes nothing. If usage cannot be read, enforcement stays as it was: a telemetry outage never lifts or applies a limit by accident. Raising or clearing a cap lifts the action within a tick.

At rollover the counters reset and any throttle or pause is lifted. The History list keeps what happened in earlier months.

## Set a cap

Dashboard: open the app, then the **Budget** section. It shows usage bars, the alert levels, the action picker and the history.

CLI:

```bash
levelrail-cli budget set web --egress 500GiB --cost 40 --action throttle --throttle-rps 5
levelrail-cli budget show web --history
levelrail-cli budget clear web
```

`set` changes only the flags you pass and keeps the rest. Sizes take `GiB` (binary) or `GB` (decimal), and `--egress 0` removes the egress limit. `budget set project|team ID --monthly N` still manages alert-only [project and team budgets](/usage-and-cost).

`app.yaml`:

```yaml
services:
  web:
    budget:
      egress: 500GiB
      cost: 40
      action: throttle
      thresholds: [50, 80, 100]
```

A deploy that carries a `budget:` block saves it. A deploy without the block leaves a dashboard or CLI configured cap alone.

## API

`GET`, `PUT` and `DELETE /api/v1/apps/{name}/budget`, and `GET /api/v1/apps/{name}/budget/history`. Reading needs the read ability; changing or clearing a cap needs the deploy ability, because a cap can pause an app. See the [API reference](/api-reference).

## Limits

- Caps are per app. Project and team budgets remain alert-only.
- Egress is the HTTP response bytes ingress served for the app. Traffic that does not pass through ingress (database ports, direct TCP) is not counted.
- Usage is checked every two minutes from cached telemetry, so a sharp spike can pass the limit before enforcement lands.
- Static sites and databases have no cap.
