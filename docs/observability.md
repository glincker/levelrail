# Observability: metrics, logs, and alerting

Levelrail's observability is built into the core, not bolted on as an afterthought. Every app and database you deploy automatically emits metrics and logs; no configuration is required to start seeing what your infrastructure is doing. This page covers what gets collected, how to query it, how to set up alerts, and how to integrate with external monitoring tools.

## Metrics: automatic collection at 15-second resolution

Every container Levelrail manages has seven metrics collected every 15 seconds and retained for 15 days by default:

- `cpu_percent`: CPU usage as a percentage of the container's limit
- `memory_usage_bytes`: current RSS memory in bytes
- `memory_limit_bytes`: the container's memory limit in bytes
- `network_rx_bytes`: bytes received on the container's network interface
- `network_tx_bytes`: bytes transmitted
- `disk_read_bytes`: bytes read from storage (cumulative, ever-increasing)
- `disk_write_bytes`: bytes written to storage (cumulative, ever-increasing)

Metrics are collected from the Docker Engine API's `/containers/{id}/stats` endpoint. Because collection is event-driven (not a polling loop), it has negligible idle cost.

### Querying metrics from the dashboard

Open the app or database detail page, scroll to the Metrics section, and click the graph to open the metrics viewer. Select a metric from the dropdown to see its history over the last hour (the default window). To change the time range, edit the dates at the top. To overlay deploy attempts on the graph as vertical markers (green for successful, red for failed), the dashboard fetches deploy history automatically and colors the markers by outcome.

### Querying metrics via the HTTP API

```
GET /api/v1/apps/{name}/metrics?metric=cpu_percent&from=2025-01-15T00:00:00Z&to=2025-01-15T01:00:00Z&step=60s
GET /api/v1/databases/{name}/metrics?metric=memory_usage_bytes
GET /api/v1/nodes/{id}/metrics?metric=disk_write_bytes
```

Query parameters:
- `metric` (required): one of the seven metric names above
- `from` (optional): RFC3339 timestamp, default is one hour ago
- `to` (optional): RFC3339 timestamp, default is now
- `step` (optional): a Go duration string like "60s" or "5m" for time-series aggregation; omit or set to "0" for raw unaggregated samples

### Retention and cleanup

The default retention window is 15 days. To change it, set the `APP_METRICS_RETENTION` environment variable on the control plane (a Go duration string, e.g. `APP_METRICS_RETENTION=7d` for seven days). A background job runs hourly to delete samples older than the retention window; there is no manual sweep command needed.

To inspect the retention and collection interval in a running control plane, check the startup logs: they log the exact configuration under "telemetry: metrics collection enabled" (or "not configured" if disabled).

## Logs: node-local storage with full-text search

Container logs are read from the Docker container's stdout and stderr streams, chunked, compressed, and stored locally on the node where that container is running. Unlike centralized logging (which creates overhead even when you never query it), node-local storage means idle log cost is near zero.

### Log storage and lifecycle

Logs are retained for 7 days by default (configurable via `APP_LOGS_RETENTION`). A background job sweeps every hour, deleting the oldest chunks. Full-text search is indexed on-disk and runs fast even on large log stores; there is no separate indexing pipeline to maintain.

### Querying logs from the dashboard

Open any app detail page and click the Logs tab. The viewer shows the most recent 100 entries by default. Use the search box to filter by keyword (searches all text in every log line). To change the time range, click the date picker at the top left. The dashboard fetches deploy history in the background; if a deploy was running when a particular log line was written, that line shows a small deploy icon you can hover for details.

### Querying logs via the HTTP API

```
GET /api/v1/apps/{name}/logs?q=error&from=2025-01-15T00:00:00Z&to=2025-01-15T01:00:00Z
GET /api/v1/databases/{name}/logs?q=statement
```

Query parameters:
- `q` (optional): full-text search phrase; omit to return every line in the range
- `from` (optional): RFC3339 timestamp, default is one hour ago
- `to` (optional): RFC3339 timestamp, default is now

### Querying logs from the CLI

```
levelrail-cli apps logs my-app
levelrail-cli apps logs my-app --since 24h
levelrail-cli apps logs my-app --q "error" --since 1h
levelrail-cli apps logs my-app --from 2025-01-15T00:00:00Z --to 2025-01-15T01:00:00Z
levelrail-cli apps logs my-app --tail 50
```

Flags:
- `--since` (default "1h"): how far back to search (a Go duration, e.g. "2h", "30m")
- `--q`: full-text search phrase
- `--from`, `--to`: RFC3339 timestamps (override `--since`)
- `--tail`: show only the last N entries (applied client-side after fetching)
- `--json`: output as a JSON array instead of a formatted table

## Alerts: threshold rules, crashloop detection, and platform-wide monitoring

Levelrail includes a built-in alert engine that evaluates eight rule kinds on a 30-second interval. When a rule fires, it sends a notification to a channel you configure (Slack, email, webhook, etc.); when the rule resolves (the condition goes false again), it sends a resolved notification so you know the issue is cleared.

### Eight rule kinds

**Threshold rules** monitor a single metric over a time window. When the metric goes above (or below) a threshold for longer than a debounce duration, the rule fires.

Example: alert when an app's CPU usage stays above 80% for 5 minutes.

**Crashloop rules** fire when a container restarts N times within a rolling time window, alerting you to a service stuck in a failing loop. When the rule fires, the last 200 lines of the container's logs are included in the notification so you can see the error without leaving your chat client.

Example: alert after 3 restarts in 10 minutes.

**Certificate expiry rules** fire when any certificate on the control plane is within 7 days of expiring, checked platform-wide (not per-app). The rule kind runs once regardless of how many apps you configure it on; no per-app noise.

**Patch status rules** fire when any node has security patches pending from the OS package manager (Linux apt, yum, or similar), checked platform-wide. Helps you avoid surprises when a newer CVE lands and no one remembers to reboot.

**Node disk space rules** fire when any node's disk usage goes above a configured threshold (default 80%), checked platform-wide.

**Node resource usage rules** fire when any node's summed CPU or memory usage goes above a configured threshold (default 85% CPU, 80% memory), checked platform-wide.

**Scheduled task failure rules** fire when one of an app's scheduled (cron) tasks fails N times in a row, including the command that failed and the failure count in the notification.

**Domain health rules** watch every domain configured on an app for DNS consistency. They fire when a CNAME repoint is detected, a required DNS record is missing, or a domain's DNS no longer resolves to the control plane's ingress address. Useful for catching accidental domain misconfigurations.

### Setting up an alert rule from the dashboard

Navigate to an app detail page, scroll to Alerts, and click Create. Choose a rule kind from the dropdown, fill in the threshold/metric/etc, pick a notification channel, and click Save. The rule starts evaluating immediately.

Platform-wide rules (cert expiry, patch status, disk space, node resource usage) can be created from the Settings page.

### Setting up alert rules via CLI

```
levelrail-cli apps alerts create my-app --kind threshold --metric cpu_percent --comparator ">" --threshold 80 --for-duration 5m --notify slack

levelrail-cli apps alerts create my-app --kind crashloop --restart-count-threshold 3 --restart-window 10m --notify email

levelrail-cli apps alerts list my-app

levelrail-cli apps alerts delete my-app {rule-id}
```

Comparators for threshold rules: `>`, `<`, `>=`, `<=`.

For a complete list of all eight rule kinds and their flags, run `levelrail-cli apps alerts -h`.

### Notification channels: Slack, Discord, email, and more

Before you can attach a rule to a channel, create the channel itself. Levelrail supports:

- **Slack** and **Discord** webhooks (URL-based, the easiest integration)
- **Telegram** (requires a bot token and chat ID embedded in the notify URL)
- **Email** (requires an SMTP server configured on the control plane)
- **Pushover** (mobile push notifications)
- **PagerDuty** (enterprise alerting and incident management)
- **Microsoft Teams** (webhooks)
- **Generic webhook** (POST JSON to any URL that accepts it)

To add a channel:

```
levelrail-cli channels create --name "Team Slack" --kind slack --notify-url https://hooks.slack.com/...

levelrail-cli channels create --name "On-call" --kind pagerduty --notify-url {routing-key}

levelrail-cli channels create --name "Admin email" --kind email --notify-url admin@example.com

levelrail-cli channels list

levelrail-cli channels test {channel-id}

levelrail-cli channels delete {channel-id}
```

To set up a Slack webhook, go to your Slack workspace, create a new Incoming Webhook (under Slack's App Directory, Incoming Webhooks), and copy the webhook URL. The same pattern works for Discord and Teams.

For email, the control plane must be running with an SMTP server configured (environment variables `APP_SMTP_HOST`, `APP_SMTP_PORT`, etc.; see the control plane startup output for the exact list).

Test a channel before attaching rules to it with `levelrail-cli channels test {id}`. This sends a real test notification so you can verify the connection works and the message format is as expected.

### Crashloop detection detail

When a Levelrail-managed container fails and Docker restarts it, Levelrail's crashloop detector watches the restart count. If a container reaches N restarts within a rolling window (default: 3 restarts within 10 minutes), a crashloop alert fires immediately with the last 200 lines of the container's logs embedded in the notification. The logs are fetched from the node's local log store, so the alert includes the actual error without requiring you to SSH in or open a dashboard.

## Integration with external monitoring: Prometheus remote read

If you use Prometheus or Grafana for metric storage and visualization, you can point them directly at Levelrail's metrics API. Levelrail exposes a Prometheus-compatible remote read endpoint:

```
POST /api/v1/prometheus/read
```

Configure this URL in Prometheus's `remote_read` section (or as a custom data source in Grafana), and queries automatically fan out to Levelrail's metric store. Every sample stored locally gets the prefix `levelrail_` (e.g. `levelrail_cpu_percent`), so metric names don't collide with other exporters.

Example Prometheus config:

```yaml
remote_read:
  - url: "http://localhost:8080/api/v1/prometheus/read"
    read_recent: true
```

The endpoint requires API authentication (the same token you use for the HTTP API); Prometheus must send it in the `Authorization: Bearer {token}` header (see Prometheus documentation for `remote_read.authorization`).

## Architecture: node-local storage, federated queries

Package: `internal/telemetry` (metrics collection and storage); `internal/alerting` (rule evaluation and notifications).

Metrics and logs are collected and indexed on the node where the container runs, not shipped to a central store. A background collector polls every running container's stats 15 times per minute (every 15 seconds), writing samples into a local SQLite database. Log entries are streamed from Docker's container logs API, chunked, compressed, and stored in the same database. Both samples and logs are indexed by resource ID (e.g. "service:web") so queries are fast even with months of history.

When you query metrics or logs from the dashboard or API, the query fans out to every node and merges results. In a single-node setup today, this is a no-op; in multi-node setups, it aggregates across machines transparently.

Alert rule evaluation is centralized on the control plane: every 30 seconds, each enabled rule queries the telemetry system (which handles the fan-out internally) and tests the condition. When a rule transitions from resolved to firing (or vice versa), the engine batches the notification and sends it to the configured channel.

This design keeps idle cost low (no constant shipit of telemetry to a central store) and query latency fast (no network hop to a separate metrics database). The tradeoff is that retention is limited by local disk space; nodes with high-cardinality metric sources will fill their disks faster than nodes with a few long-running services. The 15-day default retention and the ability to tune it per-node mitigates this.
