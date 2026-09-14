# Observability: metrics, logs, and alerts

Node-local metrics and log storage, a federated query layer, and an
alert engine on top of both. Packages: `internal/telemetry` (storage and
query), `internal/alerting` (rule evaluation and notification), and the
handlers in `internal/api/metrics.go`, `node_metrics.go`,
`database_metrics.go`, `logs.go`, `live_logs.go`, `logs_download.go`,
`app_resource_usage.go`, `alerts.go`, `notification_channels.go`.

## Why node-local, not a central store

Every other platform in this category (and every hosted observability
vendor) centralizes: ship every metric and log line to one place, index
it there, query it there. That is also exactly what makes those systems
heavy at idle. A control plane sitting between you and every sample
means write amplification for every container on every node, and a
central index that keeps growing whether or not anyone ever queries it.

This platform keeps metrics and logs where they are collected: each
node's own SQLite-backed telemetry store (the same `modernc.org/sqlite`
already used for the control plane's own state). The control plane
queries agents on demand instead of ingesting continuously. Right now
there is exactly one node (the control plane and the agent share a
process, see `internal/agent`'s in-memory transport), so "federated
query" fans out to a single source, but the query interface
(`TelemetryQuerier` in `internal/api/metrics.go`) is already the shape
Phase 3's real multi-node federation needs: nothing here changes when a
second node shows up, only how many sources `internal/telemetry`'s
querier has to ask.

## How it actually works

`internal/telemetry.Collector` samples every running container's Docker
stats every 15 seconds (`metricsCollectionInterval` in
`cmd/levelrail/main.go`) and writes them into the local metrics store
under a resource ID: `service:<name>` for an app, `database:<name>` for
a managed database, `node:<id>` for real per-node readings like disk
usage. The same resource-ID scheme is what `internal/alerting`,
the log store, and the resource-usage ranking all key off, so an app's
metrics, logs, and alert rules are always found under the one identifier
its reconciler already uses.

Logs work the same way: `internal/telemetry`'s log collector reads each
container's Docker log stream directly (not the json-file driver's raw
files), chunks and compresses it, and indexes it for full-text search in
the same per-node store. A `LogBroadcaster` fans out each line to any
live SSE subscriber at the same time it is written to the store, which
is what makes live tailing and historical search two views over the
same pipe rather than two separate systems.

Retention is a fixed sweep, not query-time filtering: `runMetricsRetentionSweep`
and `runLogsRetentionSweep` (`cmd/levelrail/main.go`) delete anything
older than the retention window once an hour. Both windows default to
15 days and are independently overridable:

- `APP_METRICS_RETENTION` (Go duration string, e.g. `"360h"`, default 15 days)
- `APP_LOGS_RETENTION` (same shape, default 15 days)

The sweep interval itself (one hour) is not configurable; only the
retention window is, per the "no hardcoded thresholds" rule.

## What gets collected

**Per app or database** (`service:<name>` / `database:<name>`), the 7
metrics `internal/telemetry`'s collector actually writes today, matching
`web/src/types/metrics.ts`'s `MetricName` one-to-one:

- `cpu_percent`
- `memory_usage_bytes`
- `memory_limit_bytes`
- `network_rx_bytes`
- `network_tx_bytes`
- `disk_read_bytes`
- `disk_write_bytes`

**Per node** (`node:<id>`), `GET /api/v1/nodes/{id}/metrics` answers two
genuinely different kinds of "node-level" reading:

- A **sum across every service currently placed on that node** for
  `cpu_percent`, `memory_usage_bytes`, `network_rx_bytes`,
  `network_tx_bytes`, `disk_read_bytes`, `disk_write_bytes`. This is not
  a read of the host's real free/total CPU or memory: `internal/agent`
  has no host-level stats collection today, so it is honestly a sum of
  already-collected per-container samples, not true host utilization.
  `memory_limit_bytes` is deliberately excluded from this sum: an
  unconstrained container's reported limit approximates the host's
  total memory, so summing it across N containers would multiply that
  number by N instead of approaching host capacity.
- A **real per-node reading** for disk usage and OS patch counts
  (`disk_used_bytes`, `disk_total_bytes`, `os_patches_available`,
  `os_security_patches_available`), collected directly by
  `HostDiskCollector`/`HostPatchCollector`, no summing involved.

The response includes `resource_count`: how many placed services
actually contributed a sample in range, not how many are placed on the
node in total (for the summed metrics), or `1`/`0` for whether the host
reading itself has data (for the real per-node metrics).

**Not collected today, called out rather than faked**: request rate,
response time percentiles, error rate, container restart count, and
build duration. These are in section 4.8's required list, but none has
a collector behind them yet (restart count/build duration/deploy
frequency were deferred as their own follow-up; request rate/response
time/error rate need an ingress-layer hook the embedded Caddy driver
doesn't expose yet). `MetricsDashboard.tsx` renders this as a plainly
labeled "not yet collected" list rather than an empty or fabricated
chart. Deploy frequency is the one exception that got closed without a
new collector: it's computed client-side from the real deploy-attempts
history the deploy markers overlay already reads.

## Dashboard pages

- **`/apps/$name/metrics`**: `MetricsDashboard`, per-app charts for the 7
  collected metrics, with deploy attempts overlaid as colored reference
  lines on each chart (green succeeded, red failed, gray running) so
  "which deploy caused this" is a glance, not a cross-reference.
- **`/databases/$name/metrics`**: `DatabaseMetricsDashboard`, the same
  chart set scoped to a managed database's own resource ID.
- Node metrics: `NodeMetricsDashboard`, the sum-across-placed-services
  view plus the real disk/patch readings, per node.
- **`/apps/$name/logs`**: two tabs over the same resource, `Live`
  (`LiveLogViewer`, the default) and `Search` (`LogSearchPanel`,
  historical full-text search), both scoped to the app's running
  container(s). Distinct from `/apps/$name/deploys/$deployId/logs`,
  which tails one specific build/deploy attempt's own output rather than
  the app's ongoing container logs.
- **`/databases/$name/logs`**: the same live/search pair for a managed
  database (`LiveDatabaseLogViewer`, `DatabaseLogSearchPanel`).
- **Dashboard overview**: `TopResourceConsumers`, ranks every app by its
  latest CPU/memory/network reading (toggle between the three), backed
  by `GET /api/v1/apps/resource-usage`. Renders nothing at all when
  telemetry isn't configured or no app has ever reported a sample,
  rather than showing a broken or empty panel on every load.
- **`/apps/$name/alerts`**: `AlertRulesPanel`, list/create/edit/delete
  for one app's alert rules, plus each rule's current firing state.
- **Settings -> Notification channels**: `NotificationChannelTable`,
  connect/edit/delete/test a channel and view its delivery history.

## Live tailing vs stored search vs download

These are three different reads over the same underlying log store, not
three separate log systems:

- **Live tail** (`GET /api/v1/apps/{name}/logs/stream`, SSE): opens with
  a short backfill (the last 5 minutes, capped at 200 lines, oldest
  first) so the view isn't blank on open, then streams every new line as
  `LogCollector` receives it from Docker. The handler subscribes to the
  live broadcaster *before* running the backfill query specifically to
  avoid a gap: subscribing second could silently drop a line that
  arrived between "query the store" and "start listening."
- **Stored search** (`GET /api/v1/apps/{name}/logs`): a real
  request/response query over what's already persisted, filtered by
  `from`/`to` (RFC3339, default the last hour) and an optional `q`
  full-text phrase. This is what "why was this app slow at 3am last
  Tuesday" actually queries.
- **Download** (`GET /api/v1/apps/{name}/logs/download`): the same
  `from`/`to`/`q` filters as stored search, but returns a plain-text
  file attachment (`Content-Disposition: attachment`) instead of JSON,
  capped at the 5,000 most recent matching lines, for pulling a copy
  into a support ticket or an archive. There is no CLI command for this
  one; it's a browser download today (`curl` with the same query params
  and a bearer token works too).

Database logs (`/api/v1/databases/{name}/logs`,
`/api/v1/databases/{name}/logs/stream`) mirror the app endpoints
exactly, same query params, same SSE shape, different resource ID
prefix (`database:` instead of `service:`). There is no download
endpoint for database logs today, only apps.

## External log drains

`PUT /api/v1/apps/{name}/log-drain` forwards an app's container log
stream to an external HTTP endpoint or syslog target, **in addition to**
(never instead of) the node-local store: a drain taps the same
`LogBroadcaster` a live SSE viewer subscribes to, it does not replace
what gets written locally. Configure it from the app's log-drain card in
the dashboard or `levelrail-cli apps log-drain set`. Clearing a drain
(`DELETE`) only stops the external forward; historical search and live
tail on this control plane are unaffected either way.

## Resource-usage ranking

`GET /api/v1/apps/resource-usage` answers "what is every app doing right
now" in one call instead of one query per app, which is exactly the N+1
shape section 4.12 rules out for page load. It returns every app that
exists, including ones telemetry has no sample for yet (a freshly
deployed app is a real zero-usage row, not a missing one), with each
field (`cpu_percent`, `memory_usage_bytes`, `memory_limit_bytes`,
`network_rx_bytes`, `network_tx_bytes`) present only when a sample has
actually been recorded for that app. One `LatestByMetric` call per
metric backs this, not one query per app.

## Alert rules

Eight rule kinds, one shared table (`alert_rules`), one evaluation loop
(`internal/alerting.Engine`) ticking every 30 seconds
(`alertEvaluationInterval`, fixed, not env-configurable). Each rule
tracks its own pending/firing state and only notifies on a firing or
resolved *transition*, never on every tick a rule stays in the same
state, so a channel doesn't get trained to be ignored.

| Kind | Scope | What it watches | Key fields |
| --- | --- | --- | --- |
| `threshold` | one app's own metric | latest value of `metric` vs `threshold`, debounced by `for_duration` | `metric`, `comparator` (`>`, `<`, `>=`, `<=`), `threshold`, `for_duration` |
| `crashloop` | one app | container restarts within a rolling window | `restart_count_threshold`, `restart_window` |
| `cert_expiry` | platform-wide | every stored TLS certificate approaching or past expiry, or stuck mid-renewal | none required |
| `patch_status` | platform-wide | every node's pending OS security patch count | none required (threshold is a control-plane default/env var, not a rule field) |
| `node_disk_space` | platform-wide | every node's disk-used percentage | none required |
| `node_resource_usage` | platform-wide | every node's summed placed-container CPU and memory | none required |
| `scheduled_task_failure` | one app's own scheduled task | consecutive failed runs of one task | `scheduled_task_id`, `restart_count_threshold` (reused as the failure-count threshold) |
| `domain_health` | one app's own domains | a DNS check gone bad (not resolving, or resolving somewhere else) on any of the app's configured domains | `for_duration` (optional debounce) |

The four platform-wide kinds (`cert_expiry`, `patch_status`,
`node_disk_space`, `node_resource_usage`) are still created through an
app's own `/apps/{name}/alerts` URL, but that URL only decides where the
rule shows up in that app's own rule list; it evaluates every
certificate, node, or disk across the whole control plane regardless of
which app it was created under.

Thresholds for the four platform-wide kinds default sensibly (cert
expiry warning window 14 days, patch-status threshold 1 pending
security patch, disk-space threshold 90% used, node CPU threshold 80%,
node memory threshold 4 GiB) and are overridable per control plane, not
per rule, via env vars: `APP_ALERT_PATCH_STATUS_THRESHOLD`,
`APP_ALERT_NODE_DISK_SPACE_THRESHOLD_PERCENT`,
`APP_ALERT_NODE_CPU_THRESHOLD_PERCENT`,
`APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES`,
`APP_ALERT_DOMAIN_HEALTH_CHECK_INTERVAL`.

A firing `crashloop` rule attaches the last 200 lines of the
crashlooping container's own logs (from the last 15 minutes,
`crashloopLogLines`/`crashloopLogLookback` in
`internal/alerting/engine.go`) directly to the outbound notification
event. That's a real, useful payload for whoever receives the webhook,
but there is no API endpoint that returns those exact attached lines
back to the dashboard after the fact; `AlertRulesPanel` links a firing
crashloop rule to the app's own live/historical log view instead of
trying to reconstruct them.

## Notification channels

Channels are global, connect-once destinations (Settings -> Notification
channels), attached to an alert rule by `channel_id` instead of retyping
a webhook URL per rule. Seventeen `kind` values map to real payload
builders in `internal/alerting/notify.go`: `generic`, `slack`,
`discord`, `telegram`, `email`, `pushover`, `pagerduty`, `teams`,
`resend`, `ntfy`, `gotify`, `mattermost`, `lark`, `rocketchat`,
`opsgenie`, `webex`, `googlechat`. For most kinds `notify_url` is a
webhook URL; a few pack more than one credential into that same field
(Pushover's user key and app token, Resend's API key and destination,
PagerDuty's routing key, Opsgenie's API key) since the wire shape is
deliberately kept to one string field per channel. `email` is the one
kind that isn't HTTP at all: it sends through the control plane's own
configured SMTP sender (Settings -> Email, falling back to
`APP_SMTP_HOST`/`APP_SMTP_PORT`/`APP_SMTP_USERNAME`/`APP_SMTP_PASSWORD`/`APP_SMTP_FROM`
if the dashboard settings are unset), and returns a clear "email is not
configured" error if neither path is set up.

**Retries**: every HTTP-based kind (everything except `email`) shares
one send path (`postJSONWithAuth`), which retries up to 3 times with a
short exponential backoff (500ms, 1s) on a transient failure: a
transport-level error (DNS, TLS, connection refused, timeout) or a
5xx/429 response. Any other status (a malformed payload, a bad
credential, a 404'd webhook URL) fails on the first attempt with no
retry, since retrying an inherently-wrong request only delays surfacing
the real problem. `email` isn't covered: it sends through the control
plane's own SMTP client, a different transport with different failure
semantics, not yet wired into this retry path.

**Test-send** fires one real message through the channel's own kind and
URL, either before a channel is ever saved
(`POST /api/v1/notification-channels/test`, kind+notify_url in the
body) or against an already-connected one
(`POST /api/v1/notification-channels/{id}/test`). Both run synchronously
with a 10-second timeout, so an unresponsive target can't hang the
request; only the existing-channel variant records a delivery-history
row.

**Delivery history** (`GET /api/v1/notification-channels/{id}/deliveries`)
lists every recorded send for a channel, newest first, cursor-paginated
by `?before` (RFC3339 timestamp, mirroring the audit log's own cursor
shape), default 50 rows, capped at 200. It captures test sends plus real
deploy-outcome and alert-rule dispatches, which record their own history
rows directly from `internal/alerting`, not through this handler.

Deleting a channel that's still attached to a rule or a deploy-notify
target never 409s: the foreign key's `ON DELETE SET NULL` just clears
the reference instead, unlike deleting a backup target.

## Integration walkthrough

1. **Query one app's CPU over the last hour, bucketed into 5-minute
   averages**:

   ```bash
   curl -s -H "Authorization: Bearer $TOKEN" \
     "https://your-control-plane/api/v1/apps/my-app/metrics?metric=cpu_percent&step=5m"
   ```

   Response:

   ```json
   { "metric": "cpu_percent", "points": [
     { "timestamp": "2026-09-12T09:00:00Z", "value": 4.2, "count": 20 },
     { "timestamp": "2026-09-12T09:05:00Z", "value": 5.1, "count": 20 }
   ] }
   ```

2. **Search that app's logs for an error in the last day**:

   ```bash
   curl -s -H "Authorization: Bearer $TOKEN" \
     "https://your-control-plane/api/v1/apps/my-app/logs?from=2026-09-11T00:00:00Z&q=panic"
   ```

3. **Connect a Slack channel and test it**:

   ```bash
   levelrail-cli channels create --name "on-call" --kind slack --notify-url https://hooks.slack.com/services/...
   levelrail-cli channels test <id>
   ```

4. **Create a threshold alert on that app, notifying through the new
   channel**:

   ```bash
   levelrail-cli apps alerts create my-app --name "high CPU" --kind threshold \
     --metric cpu_percent --comparator ">" --threshold 90 --for-duration 5m \
     --channel-id <channel-id>
   ```

5. **Watch it fire**: `levelrail-cli apps alerts list my-app` shows
   `FIRING=true` once the condition holds for 5 minutes; a delivery row
   shows up under `levelrail-cli channels deliveries <channel-id>`.

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/apps/{name}/metrics?metric=...&from=...&to=...&step=...` | `read` |
| `GET` | `/api/v1/databases/{name}/metrics` | `read` |
| `GET` | `/api/v1/nodes/{id}/metrics` | `root` |
| `GET` | `/api/v1/apps/resource-usage` | `read` |
| `GET` | `/api/v1/apps/{name}/logs?from=...&to=...&q=...` | `read` |
| `GET` | `/api/v1/apps/{name}/logs/stream` (SSE) | `read` |
| `GET` | `/api/v1/apps/{name}/logs/download` | `read` |
| `GET` | `/api/v1/databases/{name}/logs` | `read` |
| `GET` | `/api/v1/databases/{name}/logs/stream` (SSE) | `read` |
| `GET` | `/api/v1/apps/{name}/log-drain` | `read` |
| `PUT` | `/api/v1/apps/{name}/log-drain` | `write` (sensitive) |
| `DELETE` | `/api/v1/apps/{name}/log-drain` | `write` (sensitive) |
| `POST` | `/api/v1/apps/{name}/alerts` | `write` |
| `GET` | `/api/v1/apps/{name}/alerts` | `read` |
| `PUT` | `/api/v1/apps/{name}/alerts/{id}` | `write` |
| `DELETE` | `/api/v1/apps/{name}/alerts/{id}` | `write` |
| `GET` | `/api/v1/notification-channels` | `read` |
| `POST` | `/api/v1/notification-channels` | `write` |
| `PUT` | `/api/v1/notification-channels/{id}` | `write` |
| `DELETE` | `/api/v1/notification-channels/{id}` | `write` |
| `POST` | `/api/v1/notification-channels/test` | `write` |
| `POST` | `/api/v1/notification-channels/{id}/test` | `write` |
| `GET` | `/api/v1/notification-channels/{id}/deliveries?limit=...&before=...` | `read` |

## CLI

```bash
levelrail-cli apps metrics <name> --metric NAME [--since 1h | --from RFC3339 --to RFC3339] [--step 60s]
levelrail-cli databases metrics <name> --metric NAME [--since 1h | --from ... --to ...] [--step 60s]
levelrail-cli nodes metrics <id> --metric NAME [--since 1h | --from ... --to ...] [--step 60s]
levelrail-cli apps resource-usage

levelrail-cli apps logs <name> [--since 1h | --from ... --to ...] [--q PHRASE] [--tail N]
levelrail-cli apps logs <name> --follow

levelrail-cli apps log-drain get <name>
levelrail-cli apps log-drain set <name> --type http|syslog --target TARGET [--disabled]
levelrail-cli apps log-drain clear <name>

levelrail-cli apps alerts list <app>
levelrail-cli apps alerts create <app> --name NAME --kind threshold --metric METRIC --comparator OP --threshold N [--for-duration 2m] [--channel-id ID]
levelrail-cli apps alerts create <app> --name NAME --kind crashloop --restart-count-threshold N --restart-window DURATION
levelrail-cli apps alerts create <app> --name NAME --kind cert_expiry
levelrail-cli apps alerts create <app> --name NAME --kind patch_status
levelrail-cli apps alerts create <app> --name NAME --kind node_disk_space
levelrail-cli apps alerts create <app> --name NAME --kind node_resource_usage
levelrail-cli apps alerts create <app> --name NAME --kind scheduled_task_failure --scheduled-task-id ID --restart-count-threshold N
levelrail-cli apps alerts create <app> --name NAME --kind domain_health [--for-duration 2m]
levelrail-cli apps alerts update <app> <id> --name NAME --kind KIND [flags]
levelrail-cli apps alerts delete <app> <id>

levelrail-cli channels list
levelrail-cli channels create --name NAME --kind KIND --notify-url URL
levelrail-cli channels update <id> --name NAME --kind KIND [flags]
levelrail-cli channels delete <id>
levelrail-cli channels test <id>
levelrail-cli channels deliveries <id> [--limit N]
```

`--kind` for channels accepts: `generic`, `slack`, `discord`,
`telegram`, `email`, `pushover`, `pagerduty`, `teams`, `resend`, `ntfy`,
`gotify`, `mattermost`, `lark`, `rocketchat`, `opsgenie`, `webex`,
`googlechat`.

## Not built yet (deliberate gaps)

- **No request rate, response-time percentiles, error rate, container
  restart count, or build duration metrics.** Section 4.8 requires all
  of these without configuration; only 7 of the 12 required metrics
  have a real collector today. The first three need an ingress-layer
  instrumentation hook the embedded Caddy driver doesn't expose yet.
  Deploy frequency is the one previously-missing metric that's now
  covered, computed client-side from deploy-attempt history rather than
  a new collector.
- **No true host-level node metrics** (real free/total CPU or memory).
  `GET /api/v1/nodes/{id}/metrics` sums already-collected per-container
  samples across everything placed on a node; `internal/agent` has no
  `/proc` reads or host-info message today.
- **No database placement in the node-level sum.** A database placed on
  a node contributes nothing to that node's summed CPU/memory metrics
  today, only services do.
- **No CLI command for log download.** The download endpoint exists and
  works from the dashboard or a plain `curl`, but there's no
  `levelrail-cli apps logs download` wrapper around it.
- **No download endpoint for database logs**, only apps.
- **No API endpoint returns the exact log lines a firing crashloop
  alert attached to its notification.** They go out in the webhook/email
  payload only; the dashboard links to the app's own log view instead of
  reconstructing them.
- **Alert evaluation interval (30s) is fixed**, not env-configurable,
  unlike the per-kind thresholds themselves.
- **No alert-rule-specific change history.** A create/update/delete
  against a rule is only visible in the platform's generic
  `GET /api/v1/audit-log`, the same as any other authenticated write.
