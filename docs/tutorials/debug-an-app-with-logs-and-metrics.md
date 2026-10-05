---
description: Find out why an app is failing or slow with Levelrail's built-in log search and metrics, with no Grafana or Loki to install.
---

# Debug an app with built-in logs and metrics

Most self-hosted platforms tell you to install Grafana and a log stack before you can answer "what happened?". Levelrail stores metrics and logs on each node as part of the platform, so the answer is a command away. In this tutorial you will search an app's logs, follow them live, read CPU and memory over time, and use the health score and attention list to find what needs work first.

## Before you start

- A running Levelrail instance and the CLI logged in to it ([installing](../installing.md)).
- An app that has been running for a few minutes. Any app from the earlier tutorials works.

<Steps>
<Step title="Start with what needs attention">

```bash
levelrail-cli attention
```

```text
SEVERITY  KIND    SUBJECT                          DETAIL
warning   disk    data dir                         7.0% free (70057070592 of 994662584320 bytes)
warning   doctor  Control plane disaster recovery  off-box encrypted backups are not enabled
```

This is one list of everything across the instance that needs action, such as low disk space or disabled backups. It is also the **Needs attention** card on the dashboard home.

For one app, ask for its health score:

```bash
levelrail-cli apps health-score hello
```

```text
hello: fail

Deploy health  fail   most recent deploy attempt failed
Security       warn   no issued certificate found yet for hello.example.com
Resilience     pass   health checks configured; no persistent volumes to back up
Observability  fail   no alert rules configured for this app
```

Each line names a concrete gap, so you know where to look.

</Step>
<Step title="Search the logs">

Logs are stored on the node and searchable by text and time window:

```bash
levelrail-cli apps logs hello --since 30m --q healthz --tail 3
```

```text
2026-10-05T02:32:33Z stdout 172.18.0.1 - - [05/Oct/2026:02:32:33 +0000] "GET /healthz HTTP/1.1" 404 153 ...
2026-10-05T02:32:33Z stderr 2026/10/05 02:32:33 [error] 29#29: *2 open() "/usr/share/nginx/html/healthz" failed ...
```

The flags you will use most:

| Flag | What it does |
| --- | --- |
| `--since 30m` | How far back to search. Default is one hour. |
| `--from`, `--to` | An exact RFC 3339 window, for a known incident time. |
| `--q healthz` | Full-text match on the log line. |
| `--tail 50` | Only the last N entries. |
| `--json` | Machine-readable output for scripts. |

![The log viewer with full-text search and a live tail](../assets/screenshots/logs.png)

The dashboard's log viewer reads the same store, with search and live tail.

</Step>
<Step title="Follow live">

```bash
levelrail-cli apps logs hello --follow
```

This streams new lines until you press Ctrl+C. It uses the same stream as the dashboard viewer. `--follow` cannot be combined with `--since`, `--q`, or `--tail`, because it only shows new lines.

</Step>
<Step title="Read CPU and memory">

```bash
levelrail-cli apps metrics hello --metric memory_usage_bytes --since 15m --step 60s
levelrail-cli apps metrics hello --metric cpu_percent --since 15m --step 60s
```

```text
metric: cpu_percent
TIMESTAMP             VALUE                 COUNT
2026-10-05T02:31:50Z  0.014243333397954533  3
```

Metrics are sampled every 15 seconds and aggregated into the bucket size you set with `--step`. Leave `--step` off to get the raw samples.

![An app's overview with live metrics and deploy markers](../assets/screenshots/app-overview.png)

On the dashboard the same data becomes charts with deploy markers drawn on them. That is the quickest way to answer "which deploy made it slow": the line changes where the marker is.

</Step>
<Step title="Tie it back to deploys">

```bash
levelrail-cli deployments summary
```

```text
window: 24h
building     0
ready        3
failed       1
rolled_back  2
```

A spike in the metrics that lines up with a deploy usually means that release. Roll back, then investigate:

```bash
levelrail-cli apps deploys list hello
levelrail-cli apps deploys rollback-to hello <deploy-id>
```

</Step>
</Steps>

## Add an alert so you hear about it first

The health score above flagged `Observability: fail` because no alert rules exist. Alerts can go to Slack, Discord, email, Telegram, PagerDuty, ntfy, and other channels. Start with [Observability](../observability.md) and [Email notifications](../email-notifications.md).

## Where to go next

- [Observability](../observability.md): how the node-local stores work and how retention is set.
- [Log archive](../log-archive.md): keep logs in object storage beyond the local window.
- [Zero-downtime deploys with health checks](zero-downtime-deploys-with-health-checks.md): catch the problem before it ships.
