---
title: "Monitors"
description: Experimental http and tcp uptime monitors with a bounded result history and hourly rollups, off unless APP_MONITORS_ENABLED is set.
"og:url": "https://docs.levelrail.com/monitors"
---

<Warning>Monitors are experimental. The API, CLI and storage may change between releases.</Warning>

A monitor probes an endpoint on a schedule from the control plane and records the result. This first slice has `http` and `tcp` monitors, a result history, and hourly rollups. It does not send alerts, open incidents or change the status page.

The feature is off by default. When `APP_MONITORS_ENABLED` is not true, the `/api/v1/monitors` routes answer 404 and no scheduler runs.

## Kinds

- `http`: method (`GET`, `HEAD`, `POST`, `PUT`), optional headers, accepted status codes or ranges (default 200 to 399), an optional `contains` keyword searched in the first bytes of the body, and a timeout. Redirects are not followed unless asked for, up to a server limit.
- `tcp`: a host and port. The check passes when the connection completes within the timeout.

Header values are stored encrypted with the same secret store as app secrets. They are never returned by the API or the CLI, and results keep only a short failure class such as `timeout` or `status_mismatch`, never a body or header value.

## Network safety

Every connection, including redirects, goes through the outbound address guard. Private, loopback and link local addresses are refused unless the control plane has explicitly allowed private outbound targets. A refused target shows as `blocked_address` and is rejected at create time.

## Results

Results are written in batches, with the hourly rollup updated in the same transaction. Raw results are kept for a short window and rollups for much longer. Pruning runs in small batches so it does not hold the database write lock.

```
levelrail-cli monitors create --name "Site" --kind http --url https://example.com/health
levelrail-cli monitors results mon_1a2b3c4d5e6f7a8b --window 6h
```

## Settings

All limits are environment variables on the control plane. Durations use Go syntax such as `30s` or `48h`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MONITORS_ENABLED` | `false` | Turn the feature on |
| `APP_MONITOR_WORKERS` | `4` | Concurrent probes |
| `APP_MONITOR_MIN_INTERVAL_HTTP` | `30s` | Lowest http interval |
| `APP_MONITOR_MIN_INTERVAL_TCP` | `30s` | Lowest tcp interval |
| `APP_MONITOR_MAX_TIMEOUT` | `30s` | Highest per check timeout |
| `APP_MONITOR_MAX_COUNT` | `100` | Monitors per control plane |
| `APP_MONITOR_BODY_MAX_BYTES` | `65536` | Body bytes searched for `contains` |
| `APP_MONITOR_HEADER_MAX_COUNT` | `16` | Headers per monitor |
| `APP_MONITOR_HEADER_MAX_BYTES` | `4096` | Bytes per header value |
| `APP_MONITOR_MAX_REDIRECTS` | `5` | Redirect hops when following |
| `APP_MONITOR_FLUSH_INTERVAL` | `5s` | How often results are written |
| `APP_MONITOR_BUFFER_MAX` | `10000` | Results held in memory, oldest dropped first |
| `APP_MONITOR_RAW_RETENTION` | `48h` | Raw result retention |
| `APP_MONITOR_HOUR_RETENTION` | `9600h` | Hourly rollup retention |
| `APP_MONITOR_PRUNE_INTERVAL` | `10m` | How often pruning runs |
| `APP_MONITOR_PRUNE_BATCH` | `1000` | Rows deleted per pruning step |
| `APP_MONITOR_JITTER_PERCENT` | `10` | Start spread as a share of the interval |
| `APP_MONITOR_TICK_INTERVAL` | `1s` | Scheduler resolution |
| `APP_MONITOR_RESULTS_MAX_LIMIT` | `1000` | Most raw results one request returns |
| `APP_INCIDENTS_ENABLED` | `false` | Open internal, unpublished incidents from confirmed monitor outages. Needs `APP_MONITORS_ENABLED` |
| `APP_INCIDENT_FOLD_WINDOW` | `10m` | A new outage folds into an open incident on a shared status component within this window |
| `APP_INCIDENT_REOPEN_WINDOW` | `15m` | A repeat outage of the same monitor reopens a resolved incident within this window |
| `APP_INCIDENT_SWEEP_INTERVAL` | `1m` | How often open incidents are reconciled with live monitor state |
| `APP_INCIDENT_OPEN_MAX` | `200` | Open incident cap, further outages fold into one overflow incident |
| `APP_INCIDENT_QUEUE_MAX` | `256` | Transitions buffered for the opener, extra ones are dropped and the sweep catches up |
| `APP_INCIDENT_UPDATE_BODY_MAX` | `4096` | Byte cap for an internal incident note |

## API

- `GET /api/v1/monitors`, `POST /api/v1/monitors`, `DELETE /api/v1/monitors/{id}`
- `GET /api/v1/monitors/{id}/results?window=24h&limit=100` returns recent raw results and hourly rollups.

Reading needs read access and changing needs write access on the `monitors` resource. Creating and deleting are recorded in the audit log.

### Internal incidents

With `APP_INCIDENTS_ENABLED`, `GET /api/v1/incidents`, `GET /api/v1/incidents/{id}`, `POST /api/v1/incidents` (promote an alert rule: `{"source":"alert","rule_id":"..."}`), `POST .../ack`, `POST .../resolve` and `POST .../notes` manage them. Otherwise they answer 501. Reading needs the `read:sensitive` ability because timelines carry probe detail; changes need `write` and are audited. `GET /api/v1/status-page/incidents` returns published incidents only, for every caller.

`PUT /api/v1/incidents/{id}/draft` stores the public title, body, and optionally `status`, `impact` (`none`, `minor`, `major`, `critical`) and `component_ids` (existing status page components; omit to keep the stored list). `POST .../publish` is the only way an incident reaches the public page. It needs the `updated_at` you reviewed, answers 409 if the incident changed since, and 422 if the draft title or body is empty. There is no unpublish.

From the CLI:

```
levelrail-cli incidents list [--state triggered|acknowledged|resolved]
levelrail-cli incidents show <id>
levelrail-cli incidents ack <id>
levelrail-cli incidents resolve <id>
levelrail-cli incidents note <id> --body "Rolled back the migration"
levelrail-cli incidents promote <alert-rule-id> [--title "..."]
levelrail-cli incidents draft <id> --title "Elevated errors" --body "We are investigating" --impact major --components web,api
levelrail-cli incidents publish <id>
```

`publish` fetches the incident, refuses an empty draft or an already published incident before sending anything, and sends the `updated_at` it just fetched. Pass `--updated-at` with the value from `show` to refuse if the incident changed after you reviewed it.

## Status page link (opt-in)

A monitor can open a public status page incident while it is confirmed down and resolve it on recovery. It is off by default, so existing monitors do not change.

- Turn it on with the "Open status page incidents" checkbox, `levelrail-cli monitors create --status-link`, or `levelrail-cli monitors update <id> --status-link`. The API field is `status_link` (`PATCH /api/v1/monitors/{id}`).
- The incident lists the status page components of kind monitor that target this monitor, with the impact from `APP_MONITOR_LINK_IMPACT` (minor or major, default major). Add the monitor as a component to see its state change.
- One incident per outage: a repeated down never opens a second one, and a monitor that flaps back inside the recovery period keeps the same incident.
- Confirmation: `APP_MONITOR_LINK_CONFIRM_FAILURES` (default 1) failed checks, or `--link-confirm` per monitor. Recovery: `APP_MONITOR_LINK_RECOVERY_PERIOD` (default 2m) of continuous health, or `--link-recovery` per monitor. `APP_MONITOR_LINK_SWEEP_INTERVAL` (default 30s) sets how often both are re-evaluated.
- No incident opens while an active silence or maintenance window covers the monitor, or while an operator incident is already open on its component.
- Timeline entries are written by `system` and start with "[Automated]". Open and resolve write audit rows (`monitor_incident.opened`, `monitor_incident.resolved`).
- Alert channels are notified by the existing `monitor_down` rule, unchanged.
