---
title: "Alerts"
sidebarTitle: "Alerts"
description: Alert rules, the inbox, history, silences, maintenance windows and the 18 notification channel kinds, with the dashboard, CLI and API steps for each.
"og:url": "https://docs.levelrail.com/alerts"
---

An alert is a rule the control plane evaluates, plus a notification channel it sends to when the rule fires. Everything runs inside the control plane, so no outside service is needed to evaluate rules.

## What can trigger an alert?

A rule has a kind, and the kind decides what it watches. The kinds in `internal/alerting`:

- Apps: `threshold` (a metric, a comparator and a value, held for a duration), `crashloop`, `oom_killed`, `slo_burn`, `log_pattern`, `log_level_rate`, `deploy_failed`.
- Jobs: `scheduled_task_failure`, `scheduled_task_missed`, `scheduled_task_running_long`, `job_failing`. See [Scheduled jobs](/scheduled-jobs).
- Certificates and domains: `cert_expiry`, `cert_expiring`, `cert_renewal_stalled`, `parked_domain_cert_expiry`, `domain_health`, `domain_not_resolving`, `domain_registration_expiry`, `domain_traffic_drop`.
- Nodes and resource pressure: `node_offline`, `node_disk_space`, `node_host_guardrail`, `node_resource_usage`, `node_cpu`, `node_memory`, `node_cert_expiring`, `volume_usage`, `patch_status`, `version_skew`. See [Host pressure](/host-pressure).
- Databases: `database_down`, `database_connections_high`, `database_disk_high`, `database_backup_failing`, `database_backup_stale`, `pooler_saturated`.
- Backups and exports: `backup_failed`, `backup_missing`, `backup_copy_behind`, `control_plane_backup_stale`, `restore_drill_failed`, `log_archive_stale`, `export_behind`.
- Apps: `app_unhealthy` reports a container failing its liveness probe past the failure threshold and resolves when the probe passes; scope it to one app with a `service:<app>` resource. `advisory_match` reports running images matching a feed advisory and stays quiet until you turn on the `alerts.advisory_notify` setting; `alerts.advisory_min_severity` (default `high`) sets the floor.
- Platform: `registry_refresh_failing`, `dns_credential_failing`, `email_delivery_failing`, `cleanup_failing`, `budget_threshold`.
- Uptime: `monitor_down`, which reports a down [monitor](/monitors). Monitors are experimental and off unless `APP_MONITORS_ENABLED` is true.

The dashboard rule dialog, the CLI and the API do not accept exactly the same list. The API accepts every kind above. The CLI `apps alerts create` accepts the kinds named in its `--kind` help (see [CLI reference](/cli-reference)).

## How do I create a rule from the dashboard?

Open the app's Alerts section and choose **Create alert rule**. Pick a kind, fill the fields that kind needs, attach a notification channel and save. Platform-wide kinds (for example `node_offline` or `control_plane_backup_stale`) are not tied to one app, and their resource is only a display label.

Optional per-rule settings are a severity (`info`, `warning` or `critical`), labels that silences can match, a consecutive failure count, and a flap threshold and window. Left empty, the control plane defaults apply.

## How do I create a rule from the CLI or API?

Rules belong to an app and live under `apps alerts`:

```bash
levelrail-cli apps alerts create web --name "High CPU" --kind threshold --metric cpu_percent --comparator ">" --threshold 90 --for-duration 5m --channel-id CHANNEL_ID --severity warning
levelrail-cli apps alerts create web --name "Restart loop" --kind crashloop --restart-count-threshold 3 --restart-window 5m --channel-id CHANNEL_ID
levelrail-cli apps alerts list web
levelrail-cli apps alerts update web RULE_ID --kind threshold --metric cpu_percent --comparator ">" --threshold 95
levelrail-cli apps alerts delete web RULE_ID
levelrail-cli apps alerts template web --profile-type web --channel CHANNEL_ID
```

`threshold` needs `--metric`, `--comparator` and `--threshold`. `crashloop` needs `--restart-count-threshold` and `--restart-window`. Run `levelrail-cli apps alerts create -h` for the other flags.

API, with `write` ability to change and `read` to list:

| Method and path | Purpose |
| --- | --- |
| `GET`, `POST /api/v1/apps/{name}/alerts` | List or create rules |
| `PUT`, `DELETE /api/v1/apps/{name}/alerts/{id}` | Update or delete a rule |
| `POST /api/v1/apps/{name}/alerts/{id}/silence` | Silence one rule |
| `POST /api/v1/apps/{name}/alerts/apply-template` | Apply an alert template |
| `GET /api/v1/alert-templates` | List alert templates |

A rule body carries `name`, `kind`, `channel_id`, `enabled`, `severity`, `labels` and the kind fields such as `metric`, `comparator`, `threshold` and `for_duration`. See the [API reference](/api-reference).

## Which alerts exist by default?

A set of platform-wide rules is seeded on first boot, and you can turn each on or off. They include certificate expiry at 14, 7, 3 and 1 days (`APP_DEFAULT_ALERT_CERT_DAYS` changes the days), node offline, node disk space, control plane backup stale, restore drill failed, backup copy behind, export destination failing, log archive failing, registry credential refresh failing, outbound email failing, data cleanup failing, a failed deploy, a failed database backup, a stale database backup, a database down, an app crash looping (3 restarts in 10 minutes) and an app returning many 5xx errors (5 percent or more over the request window). Existing installs get the two new rules once on upgrade; a rule you deleted or edited is never re-added or overwritten. A `monitor_down` default is added when `APP_MONITORS_ENABLED` is true.

In the dashboard they are in **Alerts, Defaults**, with a switch per alert and a reset. From the CLI:

```bash
levelrail-cli alerts defaults show
levelrail-cli alerts defaults disable            # master switch for all default alerts
levelrail-cli alerts defaults enable
levelrail-cli alerts defaults list
levelrail-cli alerts defaults disable ID
levelrail-cli alerts defaults enable ID
levelrail-cli alerts defaults reset
```

API: `GET /api/v1/alert-defaults`, `PUT /api/v1/alert-defaults/{id}`, `POST /api/v1/alert-defaults/reset`, `GET` and `PUT /api/v1/alert-defaults/settings`.

Default alerts always reach the in-app inbox, and the admin email only when email is set up. The master switch is the `alerts.defaults_enabled` runtime setting, and the crash loop and 5xx thresholds are `alerts.default_crashloop_restarts`, `alerts.default_5xx_percent` and `alerts.default_5xx_min_requests`.

## Weekly digest

One plain text mail a week with apps, deploys, restarts, certificates, backups and disk use. It is off by default. Turn it on with the `alerts.digest_schedule` runtime setting (for example `mon@08`, UTC) or `levelrail-cli alerts digest schedule mon@08`. It is sent to the admin only when email is configured. See it first with `levelrail-cli alerts digest preview`.

## Where do I see what fired?

The Alerts page has five tabs: Inbox, History, Silences, Maintenance and Defaults.

- Inbox: alerts that reached you, with per-account read state, routing and an optional daily digest. See [Alert inbox](/alert-inbox).
- History: every firing and resolution, and what happened to each notification. Outcomes are `sent`, `silenced`, `grouped`, `inhibited`, `failed`, `ratelimited`, `flapping` and `skipped`. History is kept for 30 days by default (runtime setting `retention.alert_history_days`, seeded by `APP_ALERT_HISTORY_RETENTION`).

```bash
levelrail-cli alerts history --app web --outcome failed --limit 20
levelrail-cli alerts history --changes
```

`--changes` adds what changed on the app before each firing. API: `GET /api/v1/alert-history` and `GET /api/v1/apps/{name}/alert-history`.

## How do I mute an alert?

A silence stops matching alerts from notifying while they keep evaluating and stay in history. Every filter you give must match.

```bash
levelrail-cli alerts silence web RULE_ID --for 4h --reason "planned migration"
levelrail-cli alerts silences create --for 2h --node node-1 --severity warning
levelrail-cli alerts silences list
levelrail-cli alerts silences delete ID
```

Filters are rule, app, node, kind, severity, metric and label. API: `GET`, `POST /api/v1/silences` and `DELETE /api/v1/silences/{id}`.

## How do I mute alerts during planned work?

Create a maintenance window: a recurring silence defined by a cron expression, a duration and a time zone, scoped to all apps and nodes, to apps or to nodes.

```bash
levelrail-cli alerts maintenance create --name "Weekly patching" --cron "0 3 * * 0" --duration 2h --tz Europe/Berlin --scope node --target node-1
levelrail-cli alerts maintenance list
levelrail-cli alerts maintenance update ID --disabled
levelrail-cli alerts maintenance delete ID
```

API: `/api/v1/alert-maintenance-windows`. For one-off planned downtime shown on the dashboard and the public status page, use [Scheduled maintenance](/scheduled-maintenance) instead.

## How do I get notified in Slack?

Connect a Slack channel once, then attach it to rules. The same steps work for every kind.

1. Open **Settings, Notification channels** and choose **Connect channel**.
2. Pick the kind, name it and paste the destination (for Slack, an incoming webhook URL).
3. Send a test, then attach the channel to a rule.

```bash
levelrail-cli channels create --name team-slack --kind slack --notify-url https://hooks.slack.com/services/XXX
levelrail-cli channels test CHANNEL_ID
levelrail-cli channels deliveries CHANNEL_ID
levelrail-cli channels quiet-hours CHANNEL_ID --start 22:00 --end 07:00 --timezone Europe/Berlin
```

`test` sends a real message and `deliveries` lists the recorded send history. Quiet hours hold back non-critical firing notifications on one channel. A disabled channel also silences the rules attached to it. API: `/api/v1/notification-channels`, with `/test`, `/deliveries` and `/quiet-hours`.

A channel can be limited to one project by setting `project_id` when you create or update it through the API (blank means instance wide). A project channel only serves alert rules and deploy targets of apps filed under that project; attaching it to another project's app is refused with `unknown channel_id`.

## Which notification channels are supported?

The code defines 18 kinds. Maturity is from [Feature status](/feature-status).

| Kind | Label in the dashboard | Maturity |
| --- | --- | --- |
| `generic` | Generic webhook | stable |
| `email` | Email | stable |
| `slack` | Slack | beta |
| `discord` | Discord | beta |
| `telegram` | Telegram | beta |
| `pushover` | Pushover | beta |
| `pagerduty` | PagerDuty | beta |
| `teams` | Microsoft Teams | beta |
| `resend` | Resend | beta |
| `ntfy` | ntfy | beta |
| `gotify` | Gotify | beta |
| `mattermost` | Mattermost | beta |
| `lark` | Lark | beta |
| `rocketchat` | Rocket.Chat | beta |
| `opsgenie` | Opsgenie | beta |
| `webex` | Webex | beta |
| `googlechat` | Google Chat | beta |
| `webpush` | Browser push | beta |

Beta means tested against local fakes that check each vendor's documented payload shape, with no real vendor account accepting a send yet. `webpush` is tested with a recording sender only, and its destination is the registered browser subscriptions instead of a URL. Only users who may see an event receive it: instance notices go to admins, and app events go to users who can read that app. `channels create` accepts every kind except `webpush`. For email providers see [Email notifications](/email-notifications).

## How does Levelrail avoid alert noise?

Several controls sit between a firing rule and the channel:

- Consecutive failures: a rule fires only after its condition holds for this many evaluations in a row. Default 1 (`APP_ALERT_CONSECUTIVE_FAILURES`), or set per rule.
- Flap detection: a rule that fires 5 times in 30 minutes is marked flapping (`APP_ALERT_FLAP_THRESHOLD`, `APP_ALERT_FLAP_WINDOW`, or per rule).
- Grouping: firing alerts for one target can be buffered into one summary. Off by default (`APP_ALERT_GROUP_WINDOW` is 0). A repeat of the same rule replaces its entry in the group.
- Rate limit: at most 30 sends per 10 minutes per target (`APP_ALERT_CHANNEL_RATE_LIMIT`, `APP_ALERT_CHANNEL_RATE_WINDOW`). Over the limit, the send is recorded as `ratelimited`.
- Inhibition: alerts for apps on a node that is offline are recorded as `inhibited`, so the node alert is the one you see.
- Sleeping apps: an app stopped by [sleep when idle](/sleep-when-idle) sends no alerts while it sleeps. A firing alert is recorded as `inhibited` ("app is asleep by design") and is sent if it is still firing after the app wakes. Turn on the runtime setting `alerts.notify_on_sleep` (Settings, Runtime, Observability card, or `levelrail-cli settings runtime set alerts.notify_on_sleep true`) to be told about sleeping apps too. Default off, applied live.
- Silences, maintenance windows and channel quiet hours, above.

Noise control state is held in memory, so a control plane restart forgets pending groups and streaks. Severity is `info`, `warning` or `critical`. The inbox hides `info` by default.

## How do alerts relate to the status page and heartbeats?

They are separate. Alerts notify you. The [status page](/status-page) is what your users see, and only monitors with the status link setting open and resolve its incidents. The [off-host heartbeat](/heartbeat) is a ping an outside service watches, so you hear about a dead control plane that cannot send alerts itself. [External monitoring](/external-monitoring) compares the three.

Deploy results can also go to a channel with `levelrail-cli apps deploy-notify-targets create <app> --channel-id ID`, and signed JSON events go out through [Event webhooks](/event-webhooks).

## Related

- [Alert inbox](/alert-inbox)
- [Observability](/observability)
- [Notification channel email setup](/email-notifications)
- [Monitors](/monitors)
