---
title: "Cost explorer and budgets"
description: "Explore cost by app, project, tag, node or kind, see a month end forecast, and set budgets that alert at thresholds or pause and throttle apps."
"og:url": "https://docs.levelrail.com/cost-explorer"
---

The Cost explorer builds on [Usage and cost](/usage-and-cost). It uses the same prices and the same daily snapshots, and adds a trend view, a forecast with a range, top movers and named budgets. Like the rest of that page it is showback of your own prices, not an invoice.

## Exploring cost

Open **Usage and cost** in the sidebar, then the **Cost explorer** tab. Pick how to group the cost:

| Group by | Meaning |
| --- | --- |
| App | One series per app. Databases are left out. |
| Project | One series per project, plus "Unassigned". |
| Tag | One series per resource tag. A resource with several tags counts in each, so tag totals can exceed the total. Untagged resources are grouped together. |
| Node | One series per node, from where each app or database runs. |
| Kind | Apps against databases. |

Daily granularity covers month to date, the last 30 days or last month. Monthly granularity covers the last 3, 6 or 12 calendar months, with the current month included so far. The chart shows the top 10 groups and folds the rest into "Other". Each group is compared with the previous period of the same length.

**Top movers** lists the groups whose cost grew most against the previous period. Only increases are listed.

## Forecast and range

The month end forecast projects the month from the mean of the most recent full days (up to seven), so a spike early in the month does not dominate. The range is an 80 percent band from the spread of those days: the remaining days are treated as independent, so the band widens with the square root of the days left and is never below what is already spent.

Confidence is reported with it. It is low with fewer than three full days (the range is then a fixed 25 percent either side of the projected part), medium when daily spend is reasonably steady, and high with at least five steady days. Treat it as a planning aid: a deploy that changes usage will move it.

## Budgets

A budget is a named monthly amount for a project, a tag or an app.

- **Thresholds**: alert levels in percent of the monthly amount, default 50, 80 and 100. Each fires once per month. The highest level reached is stored, so a restart or a missed check never repeats an alert, and the count resets at the start of the next UTC month.
- **Action at 100 percent**: `none` (alert only), `throttle` or `pause`. These are the same actions per-app [spend caps](/spend-caps) use, applied through the same ingress path. Nothing is ever deleted. The action covers the apps in scope (databases count toward spend but are never paused). It is lifted as soon as the budget is raised above current spend, the budget is deleted, or the month rolls over. If a spend cap and a budget both act on an app, pause wins over throttle, and the stricter throttle rate wins between two throttles.
- **Evaluation**: a ticker re-derives every budget from stored state and current spend every two minutes. If usage data is unavailable it leaves enforcement as it is rather than lifting or applying a limit by accident.

Alerts go through the existing `budget_threshold` alert rule, so they reach the channels you already set up (email, Slack, webhook). Create one under Alerts if you have none. Without a rule, the budget still tracks thresholds and applies its action, but nobody is notified.

### Fixing a breach

A budget that is at a warning level, over, or forecast to overrun shows buttons on its row:

- **Raise budget** sets the amount to a suggestion that covers the forecast range.
- **Pause app** stops one of the apps in scope after a confirmation. Start it again from the app page.
- **Right-size apps** opens the Resource advisor for limits that are too generous.

Budgets span the whole instance, so creating, changing or deleting one needs an instance admin. Team scoped users can still use the explorer, which shows only their own resources.

## Command line, API and MCP

```
levelrail-cli costs explore --by tag --period 30d
levelrail-cli costs explore --granularity monthly --months 6 --by project
levelrail-cli costs budgets list
levelrail-cli costs budgets create --name prod --scope tag --value prod --monthly 200 --thresholds 50,80,100 --action pause
levelrail-cli costs budgets set BUDGET_ID --monthly 300
levelrail-cli costs budgets delete BUDGET_ID
```

Every command takes `--json`. A project scope accepts the project id or its name.

| Route | Purpose |
| --- | --- |
| `GET /api/v1/costs/explore` | `group_by=app\|project\|tag\|node\|kind`, `granularity=daily\|monthly`, `period=mtd\|30d\|last_month\|custom`, `months=1..12` |
| `GET`, `POST /api/v1/costs/budgets` | List with status, create |
| `PUT`, `DELETE /api/v1/costs/budgets/{id}` | Update (only the fields sent change), delete |

The MCP server exposes read-only `cost_explorer` and `cost_budgets` tools.

## Limits

- Cost is only as accurate as your prices and the measured usage, see [Usage and cost](/usage-and-cost).
- A budget enforces on apps that exist when it is evaluated. An app added to a tag later is picked up on the next check.
- The forecast assumes recent daily spend continues. It cannot see a planned deploy or a traffic event.
