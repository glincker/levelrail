---
title: "Spend caps"
description: "Set a monthly egress and cost limit per app, get alerts at 50, 80 and 100 percent, and choose whether the app is throttled or paused when the limit is reached."
"og:url": "https://docs.levelrail.com/spend-caps"
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

## Block new deploys

Set the guard mode to `block_deploys` and a capped app refuses **new** deploys once it reaches 100 percent. The release that is running keeps serving and rollbacks are not blocked. The refusal names the cap and the percentage, and the deploy fails before anything is built.

The guard works on its own or together with `throttle` or `pause`. It also works for a project cap (see below), where it covers every app in the project.

A root user can override the guard for a short window: the Spend card shows an **Allow deploys** button, or use `spend caps override app NAME --reason TEXT`. A reason is required, the override expires on its own (one hour by default), and it is recorded in the app's history with the user's name. Only a root token can turn `block_deploys` on or off.

## Forecast

Each cap shows a projected month end position next to the usage bar. Cost is projected from the recent daily rate with a range, egress from the month to date rate. A red projection means the cap will be reached before the month ends at the current pace.

## Project caps

A project can carry one monthly cost cap that covers every app in it, with the same alert levels, action and deploy guard. Open the project page and use the **Spend** card, or `levelrail-cli spend caps set project NAME --cost 200`. Project caps are named budgets under the hood, so they also appear under `costs budgets` and on the Costs page.

## Preview a pause

**Preview a pause** (or `spend caps dry-run app|project NAME`) lists the apps in scope, their replica counts and the estimated saving until month end. The saving is the recent compute and egress run rate times the days left. Storage and idle node cost are not counted because stopping an app does not remove them. The preview changes nothing: no app is stopped, scaled or paused.

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

Spend caps are also under their own command, which covers both scopes:

```bash
levelrail-cli spend caps list
levelrail-cli spend caps set app web --cost 40 --warn 80,100 --mode block_deploys
levelrail-cli spend caps set project acme --cost 200 --mode block_deploys
levelrail-cli spend caps status app web
levelrail-cli spend caps dry-run project acme
levelrail-cli spend caps override app web --reason "checkout hotfix"
levelrail-cli spend caps rm project acme
```

A deploy that carries a `budget:` block saves it. A deploy without the block leaves a dashboard or CLI configured cap alone.

## API

`GET`, `PUT` and `DELETE /api/v1/apps/{name}/budget`, and `GET /api/v1/apps/{name}/budget/history`. `GET` returns the status, the month end `forecast` and the deploy `guard` state. `POST /api/v1/apps/{name}/budget/dry-run` previews a pause, and `POST /api/v1/costs/budgets/{id}/dry-run` does the same for a project budget. `POST /api/v1/apps/{name}/budget/override` takes a `reason` and needs a root token. `GET /api/v1/costs/app-caps` lists every app cap. Reading needs the read ability; changing or clearing a cap needs the deploy ability, because a cap can pause an app, and changing the guard mode needs root. See the [API reference](/api-reference).

## Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SPEND_WARN_PERCENTS` | `50,80,100` | Alert levels used when a cap does not set its own, for example `80,100`. |
| `APP_SPEND_OVERRIDE_TTL` | `1h` | How long a root override lets deploys through, up to seven days. |

## Limits

- A project cap alerts and can block deploys, throttle or pause its apps. Team budgets remain alert-only.
- There is no automatic stop or scale to zero. The pause preview shows what that would do; a real pause needs its own design review.
- Egress is the HTTP response bytes ingress served for the app. Traffic that does not pass through ingress (database ports, direct TCP) is not counted.
- Usage is checked every two minutes from cached telemetry, so a sharp spike can pass the limit before enforcement lands.
- Static sites and databases have no cap.
