---
title: "Custom dashboards"
description: Build your own views of metrics, logs, deploys, alerts and cost, share them, and keep them in version control.
"og:url": "https://docs.levelrail.com/custom-dashboards"
---

A custom dashboard is a named grid of widgets that you assemble yourself, against any app, database or node. Dashboards read the telemetry and resource endpoints the platform already has, so they add no new storage and no agents.

Open **Dashboard** in the sidebar. Your boards are tabs next to **Start** and **Metrics**, and the **Manage** menu creates, imports, renames, exports and deletes them. The tab you used last opens first; before that it is a board once you have apps, and Start before your first app. Old `/dashboards` links open the same board here. **New from a starter** offers three one click starters.

## Starters

| Starter | What it shows |
| --- | --- |
| App health | CPU, memory, request latency, error rate, alert status, recent deploys and a log tail for one app |
| Fleet overview | How many apps, databases, nodes and domains you run |
| Cost | Month to date spend by tag next to CPU and memory |

App based starters ask which app to read from. Every widget can be changed afterwards.

## Widgets

| Widget | Reads from | Notes |
| --- | --- | --- |
| Metric chart | app, database or node | CPU, memory, network, disk and restarts; uses the same chart as the app and node pages |
| Request latency | app | p95 latency and requests per second from ingress |
| Error rate | app | 4xx and 5xx share of requests |
| Log tail | app | Latest 50 lines in the time range, with an optional text filter |
| Deploy timeline | app | The app's latest timeline entries |
| Resource counts | whole fleet | Apps, databases, nodes and domains |
| Cost by tag | whole fleet | Top tags by month to date cost, when prices are set under Usage and cost |
| Alert status | app | How many alert rules are enabled |
| Node capacity | node | Disk and memory used |

## Layout and editing

Choose **Manage**, then **Edit board**, to change a dashboard. Each widget has buttons to move it earlier or later, to resize it (S is a quarter row, M a half row, L a full row) and to remove it. They are plain buttons, so everything works from the keyboard. **Add widget** picks the widget, what it reads from and, for charts, the metric.

The time range and the auto refresh interval (manual, 15 seconds, 30 seconds, a minute or 5 minutes) sit above the grid and are saved with the dashboard.

## Private, shared and default

- A dashboard is private to you by default.
- Turn on **Shared with everyone** to let every signed in operator see and edit it.
- A shared dashboard can belong to a project and be that project's **default**. Only one dashboard per project is the default, and the default is listed first.

Creating, changing and deleting dashboards needs the write ability, and each change is recorded in the audit log.

## Keep dashboards in git

Export writes a portable JSON document. Import validates it against the dashboard JSON Schema and creates a new dashboard, so a dashboard can be reviewed in a pull request and applied on another install.

```bash
levelrail-cli dashboards list
levelrail-cli dashboards create --template app-health --target web --name "Web health"
levelrail-cli dashboards export "Web health" > web-health.json
levelrail-cli dashboards import web-health.json --scope shared --project my-project
levelrail-cli dashboards show "Web health" --json
levelrail-cli dashboards delete "Web health"
```

Dashboards are identified by id or by name. All commands accept `--json`.

A minimal document:

```json
{
  "version": 1,
  "name": "Web health",
  "time_range": "1h",
  "refresh_seconds": 30,
  "widgets": [
    { "id": "cpu", "type": "metric_chart", "size": "M", "metric": "cpu_percent", "target": { "kind": "app", "name": "web" } },
    { "id": "logs", "type": "log_tail", "size": "L", "target": { "kind": "app", "name": "web" } }
  ]
}
```

Widget ids are lowercase letters, digits and dashes and must be unique inside a dashboard. A dashboard holds at most 40 widgets. Documents with an unknown widget type, size or field are rejected.

## API

| Route | Purpose |
| --- | --- |
| `GET /api/v1/dashboards` | List the dashboards you can see; `?project=` narrows to one project |
| `POST /api/v1/dashboards` | Create from a document with `scope`, `project_id` and `is_default` |
| `GET /api/v1/dashboards/{id}` | One dashboard with its document |
| `PUT /api/v1/dashboards/{id}` | Replace a dashboard |
| `DELETE /api/v1/dashboards/{id}` | Delete a dashboard |
| `GET /api/v1/dashboards/{id}/export` | The portable document |
| `POST /api/v1/dashboards/import` | Create from an exported document |
| `GET /api/v1/dashboards/templates` | The starters |

## MCP

The MCP server exposes `list_dashboards` and `get_dashboard` (read only), so an assistant can see which views an operator keeps and which resources they watch.
