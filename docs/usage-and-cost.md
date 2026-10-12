---
title: "Usage and cost"
description: "Cost showback per app, project, environment and team: allocate your own prices and server bills by measured usage, with budgets and alerts."
---

The Usage and cost page answers two questions: what does this app cost me to run, and which team is using the most. It takes the prices you enter and applies them to the CPU, memory, storage and traffic each app and database actually used, then adds the result up per app, project, environment and team.

This is showback, not an invoice. Nothing is fetched from a cloud provider and nothing is billed. The numbers are only as good as the prices you enter and the usage the platform measured.

## How a cost is worked out

Each UTC day is priced once, when it is recorded, and the result is stored. That is why history survives telemetry retention and why changing a price does not silently rewrite last month.

There are two ways to price compute.

**Split each server's monthly bill (recommended).** Enter what each server costs per month, with its vCPUs and memory. The platform divides the bill by days in the month and gives each app a share by how much of the server it used:

```
share = cpu_weight * (cores used / server vCPUs) + (1 - cpu_weight) * (memory used / server memory)
app cost for the day = daily bill * share
```

`cpu_weight` defaults to 50 percent. What is left after every app has taken its share is idle capacity. By default it is shown as **Unallocated** so you can see how much of the bill nobody used. Set the idle spread to 100 percent and it is distributed over apps by usage instead, so the report adds up to the whole bill. A value in between spreads that share and shows the rest as Unallocated.

If the apps together use more than the server, their shares are scaled down so the total never exceeds the bill.

**Price per unit of usage.** Enter a price per vCPU hour and per GiB of memory hour. An app costs `cores * hours * cpu price + GiB * hours * memory price`. Use this when there is no single bill to split. Presets turn a typical VPS size into unit prices as a starting point; they are illustrative and you should correct them.

In both modes you can also set a price per GiB of volume storage per month and per GiB sent out. Storage is the size of the app's local named volumes when the day was recorded. Egress is bytes served through the platform's ingress.

### What counts as usage

Usage is measured from the metrics the collector already keeps, every 15 seconds. For each day the platform records the average and the median (p50) CPU and memory, and how many seconds the container was actually reporting. A stopped container accrues nothing, so downtime is not charged as if it had run. Choose whether allocation uses the average or the p50 in the prices dialog.

## The report

Pick month to date, the last 30 days or last month. The page shows:

- the total, and the change against the previous period (for month to date, the same number of days at the start of last month);
- the forecast for month end, from the last few full days of spend;
- the unallocated share, when you can see instance wide figures;
- a stacked bar of daily cost by project;
- a ranked table by project, environment, team or app or database, each with its trend;
- the biggest movers against the previous period.

On an app's Resources tab a chip shows that app's cost so far this month.

Export the resource breakdown with the CSV button, the CLI or `GET /api/v1/usage/report?format=csv`.

## Budgets and alerts

A budget is a monthly amount for a project or a team, with a warning level that defaults to 80 percent. Create a `budget_threshold` alert rule (Alerts, Create rule) to be notified through your existing channels when any budget:

- reaches its warning level,
- goes over its limit, or
- is forecast to overrun this month.

A rule that is already firing notifies again only when a new level is crossed. It resolves when no budget is at any of those levels, for example at the start of the next month.

## Who sees what

Instance admins see everything and set prices. A member of a team sees only the cost of the resources their team role can read: other teams' apps, project names and totals are not in their report. The unallocated share is an instance wide figure and is hidden from team scoped callers, and so are server bills. Team admins and owners can manage budgets for their own teams; instance admins manage any.

## Command line and API

```
levelrail-cli usage report --period mtd --by team
levelrail-cli usage report --period 30d --csv > usage.csv
levelrail-cli usage settings
levelrail-cli usage settings set --mode node --node local=40:4:8 --reprice-month
levelrail-cli usage budgets set project PROJECT_ID --monthly 120 --warn-pct 80
levelrail-cli usage budgets
```

| Route | Purpose |
| --- | --- |
| `GET /api/v1/usage/report` | Report; `period=mtd\|30d\|last_month\|custom`, `from`, `to`, `format=csv` |
| `GET`, `PUT /api/v1/usage/settings` | Prices, node bills, allocation options (instance admin) |
| `GET`, `PUT /api/v1/usage/budgets`, `DELETE /api/v1/usage/budgets/{id}` | Budgets |

The MCP server exposes the report as the read-only `usage_report` tool.

Two environment variables tune the control plane: `APP_USAGE_ROLLUP_LOOKBACK_DAYS` (default 35) is how far back missing days are recorded, and `APP_USAGE_LIVE_CACHE_SECONDS` (default 60) is how long the partial current day is cached.

## Limits

- **Costs are allocations of your own numbers.** If a server bill is wrong, every cost on it is wrong by the same factor.
- **History starts when recording starts.** Days older than telemetry retention that were never recorded have no cost.
- **Repricing is explicit.** Past days keep the prices of the day they were recorded unless you choose to reprice the month to date.
- **Storage is a point in time.** A day uses the volume sizes seen when it was recorded, and only volumes on the control plane's own Docker daemon are measured.
- **Servers without a price are not costed.** The report lists them so a missing price does not look like a free server.
- **Days are UTC.** A day boundary is midnight UTC whatever your browser time zone is.
- **Shared servers have one bill.** Anything that runs on a server but is not an app or database (the platform itself, the operating system) is part of the idle share, not its own line.
