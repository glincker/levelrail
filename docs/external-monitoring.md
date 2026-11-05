---
title: "External monitoring"
description: "Watch Levelrail from outside the server with the built-in public status page and the off-host heartbeat ping, and know which one answers which question."
"og:url": "https://docs.levelrail.com/external-monitoring"
---

Levelrail sends its alerts from inside the control plane. That cannot report a dead server, so two features reach outside it. The off-host heartbeat makes the control plane call a URL you choose while it is healthy, and an outside monitor alerts when the calls stop. The public status page gives your users and your own checks a read-only view of service health. Both ship today. Neither needs a second service on your server.

## Which page do I read?

This page compares the ways to see an outage from outside the server. For the setup and variables of the off-host ping, read [Off-host heartbeat](/heartbeat). For probes the control plane runs against your endpoints, read [Monitors](/monitors) (experimental).

## Which tool answers which question

| Question | Use | What happens |
| --- | --- | --- |
| Is the server or the control plane still alive? | [Off-host heartbeat](/heartbeat) | The control plane pings your monitor every interval while healthy. |
| Did a scheduled job finish, and did it fail? | [Per-job heartbeat](/scheduled-jobs#heartbeat-pings) | The control plane pings your monitor after each job finishes. |
| What do my users see right now? | [Public status page](/status-page) | Visitors and checks read a page the control plane serves. |
| Is one URL or port up? | [Monitors](/monitors) | Experimental. The control plane probes the target. |

The heartbeat tells you the server is alive. It does not tell you which app is down, so keep your app-level alerts next to it.

## Off-host heartbeat

The control plane checks three things before each ping:

- the database answers within a short probe timeout,
- the reconcile loop completed a pass within `APP_HEARTBEAT_RECONCILE_WINDOWS` resync intervals (default `3`),
- the local node is registered. A fresh install with no local node yet skips this check.

When all three pass, it sends a ping to the URL. When one fails and you set a failure URL, it calls the failure URL instead, with a `reason` word (`store`, `reconcile` or `agent`) so the monitor can name the cause. When one fails and no failure URL is set, nothing is sent, and the monitor alerts after its grace period.

The default ping is a POST with a small JSON body:

```json
{"status":"ok","version":"1.2.3","uptime_seconds":123}
```

A failure ping uses `"status":"degraded"` and adds the `reason` field. The body holds no hostnames, app names or identifiers. Set `APP_HEARTBEAT_BODY=off` to send no body at all.

### Set it up

In the dashboard, open Settings, Notification channels, and the Off-host heartbeat card. Paste the ping URL and save. The URL is stored encrypted, and afterwards only its host is shown.

On the server, the environment variables are `APP_HEARTBEAT_URL`, `APP_HEARTBEAT_FAIL_URL` and `APP_HEARTBEAT_INTERVAL` (default `60s`, at least `5s`). When `APP_HEARTBEAT_URL` is set, the dashboard card becomes read-only. An invalid URL stops the control plane at boot with a message that names the variable.

The full variable list is on the [Off-host heartbeat](/heartbeat) page.

### Pair it with a monitor

Any monitor that accepts a dead man's switch ping works the same way:

1. Create a check or heartbeat monitor in your monitoring service.
2. Set its expected period a little above `APP_HEARTBEAT_INTERVAL`.
3. Set its grace period to a few minutes, so one slow ping does not page you.
4. Paste its ping URL into Levelrail.
5. For the failure URL, use the monitor's failure or down endpoint if it has one.

Host the monitor on a different machine or provider than the server it watches. A monitor on the same box stops with it.

### Redirects, private addresses and logs

Redirects to another host are refused, and at most three redirects are followed on the same host. Private and loopback destinations are blocked unless `APP_HEARTBEAT_ALLOW_PRIVATE=1` is set, which is useful for a monitor on your LAN. The URL never appears in logs or API responses. Only its host does.

## Per-job heartbeats

A scheduled job can ping its own URL, so a monitor alerts when a nightly job stops or fails. Set the URL per job; see [Scheduled jobs](/scheduled-jobs#heartbeat-pings).

- A successful job sends a GET to the job's ping URL.
- A failed, timed out, interrupted or container-not-running job sends a GET to the fail URL. If none is set, the fail URL is the ping URL with `/fail` added to the path, and the query string is kept.
- A skipped job, or one replaced by a newer one, sends nothing, so the monitor's grace period decides.
- A transient failure (a network error or a 5xx answer) is retried with backoff. A 4xx answer is final.
- A ping never changes the job result.

The retry limits are runtime settings. They apply without a restart, and a stored value wins over the environment variable that seeds the default:

| Setting | Default | Environment seed | Meaning |
| --- | --- | --- | --- |
| `heartbeat.job_max_attempts` | `3` | `APP_HEARTBEAT_JOB_MAX_ATTEMPTS` | Tries per ping before it is dropped. |
| `heartbeat.job_backoff_base` | `2s` | `APP_HEARTBEAT_JOB_BACKOFF_BASE` | First wait before a retry, doubled on each retry. |
| `heartbeat.job_backoff_max` | `30s` | `APP_HEARTBEAT_JOB_BACKOFF_MAX` | Cap on the wait between retries. |
| `heartbeat.job_queue` | `64` | `APP_HEARTBEAT_JOB_QUEUE` | Pings waiting to be sent. A ping that finds the queue full is dropped, and the job is not delayed. |

```bash
levelrail-cli settings runtime get heartbeat.job_max_attempts
levelrail-cli settings runtime set heartbeat.job_max_attempts 5
levelrail-cli settings runtime reset heartbeat.job_max_attempts
```

A dropped ping writes a warning to the control plane log that names the task, the execution ID and the host. The full URL is never logged.

## Public status page

The status page is off until you enable it. It lists only the components you choose, under public names you choose. A visitor sees the overall state, each component's status, a 90 day uptime bar, active and recent incidents, and scheduled maintenance. The page itself is server-rendered HTML with no JavaScript.

Turn it on and add a component from the CLI:

```bash
levelrail-cli status-page set --enable --title "Acme status" --description "Live service health"
levelrail-cli status-page components add --kind check --target https://api.example.com/health --name "API"
levelrail-cli status-page preview
```

The page is served at `/public/status` on the control plane host. The same data is available at `/public/status.json`, `/public/status.rss` and `/public/status.atom`. For a status domain of its own, use `status-page set --domain HOST`, then point that DNS record at the control plane.

Component kinds decide how status is measured:

| Kind | Target | Status |
| --- | --- | --- |
| `app` | an app name | The app's reconcile conditions. Healthy is operational, reconciling is degraded, and attention needed or stopped is an outage. |
| `domain` | a hostname | The domain DNS check. Connected is operational, and not resolving is an outage. |
| `check` | an `http` or `https` URL | A `GET` each sample. 2xx and 3xx are operational, 4xx is degraded, 5xx or unreachable is an outage. |

Uptime bars come only from samples the control plane records while the page is enabled, one per component per `APP_STATUS_PAGE_SAMPLE_INTERVAL` (default `1m`). There is no backfill, so days before you enabled the page show as "no data".

### What the public view includes

The public view is a fixed list: the page title and description, component public names, statuses, uptime figures, and the incident and maintenance text you write. It never includes check URLs, app names behind a component, hostnames, error messages, internal IDs, node names or metrics.

### Tuning and limits

Public reads are rate limited per client IP. `APP_STATUS_PAGE_RATE_LIMIT_RPM` defaults to `120`. The built view is cached for `APP_STATUS_PAGE_CACHE_TTL` (default `30s`). Each probe times out after `APP_STATUS_PAGE_CHECK_TIMEOUT` (default `8s`).

The rate limit uses the TCP peer address and does not trust `X-Forwarded-For`. Behind a reverse proxy, every visitor shares the proxy's budget, so raise the limit or cache at the proxy.

Uptime is measured from this one control plane, not from several regions, and covers only the time the page has been enabled. There are no email or webhook subscriptions to the page itself, so the RSS and Atom feeds are the push channel. Signed webhooks for admin-created subscribers are described on the [Public status page](/status-page).

## A first-week check

Do these once after setup, from a machine that is not the server:

1. Set the heartbeat URL and confirm the monitor shows a ping within two intervals. pending run
2. Stop the control plane on a test install and confirm the monitor alerts after its grace period. pending run
3. Fail the database or stall the reconcile loop on a test install and confirm pings stop, or that the failure URL is called. pending run
4. Enable the status page, add one `check` component, wait a few minutes and open `/public/status` and `/public/status.json`. pending run
5. Load `/public/status` through your custom status domain and confirm TLS is issued. pending run

The steps are not yet verified on a real host. Mark each one as done in your own runbook when you have carried it out.

## What this does not cover

- No notification to your users when the page changes, beyond the feeds.
- No per-app alerting from the heartbeat. Use your app alerts for that.
- No ping from a node when the control plane is up but a node is unhappy. Node silence pings cover only the case where a node loses contact with the control plane. See [Off-host heartbeat](/heartbeat#when-only-the-control-plane-dies).

## Related

<CardGroup cols={2}>
<Card title="Off-host heartbeat" href="/heartbeat">

Full variable list, the health gate and the node silence URL.

</Card>
<Card title="Monitors" href="/monitors">

Experimental http, tcp and tls probes run from the control plane.

</Card>
<Card title="Public status page" href="/status-page">

Components, incidents, maintenance, feeds, webhooks and branding.

</Card>
</CardGroup>
