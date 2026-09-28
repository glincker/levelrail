---
description: An opt-in public status page with operator-chosen components, 90 day uptime bars, incidents, maintenance announcements and an RSS feed.
---

# Public status page

The control plane can serve a read-only status page for your users. It is off until you turn it on, it lists only the components you choose under a public name you choose, and it never shows anything else.

## What it shows

- An overall banner: all systems operational, degraded performance, service disruption, or maintenance in progress.
- Each component with its current status and a 90 day uptime bar (one bar per UTC day).
- Active incidents and recent resolved ones (14 days), with their timeline of updates.
- Scheduled and in-progress maintenance, and recently completed maintenance (14 days).
- Links to an RSS feed and a JSON version of the same data.

The page is server-rendered HTML with no JavaScript, works in light and dark mode, and is readable with a keyboard and screen reader (status is always written as text as well as color, and each uptime bar has a text label).

## What it never shows

The public view is a fixed whitelist: page title and description, component public names, statuses, uptime numbers, and the incident and maintenance text you write. Component targets (app names, hostnames, health check URLs), error messages, IDs of internal resources, node names and metrics are never included. A privacy test fails the build if a field is added to the public payload without being reviewed.

Incident and maintenance text is escaped. Only paragraphs, line breaks, `- ` lists, `**bold**`, `` `code` `` and `http(s)` links are formatted; raw HTML is shown as text.

## Set it up

In the dashboard open **Settings, Status page**, or use the CLI:

```bash
levelrail-cli status-page set --enable --title "Acme status" --description "Live service health"
levelrail-cli status-page components add --kind app --target web --name "Website"
levelrail-cli status-page components add --kind domain --target example.com --name "Marketing site"
levelrail-cli status-page components add --kind check --target https://api.example.com/health --name "API"
levelrail-cli status-page preview
```

The page is then available at `/public/status` on the control plane host, with `/public/status.json` and `/public/status.rss` next to it.

### Component kinds

| Kind | Target | How its status is measured |
| --- | --- | --- |
| `app` | an app name | the app's reconcile conditions: healthy is operational, reconciling is degraded, attention needed or stopped is an outage |
| `domain` | a hostname | the existing domain DNS check: connected is operational, not resolving or resolving elsewhere is an outage |
| `check` | an `http` or `https` URL | a `GET` every sample interval: 2xx and 3xx operational, 4xx degraded, 5xx or unreachable an outage |

HTTP checks go through the same SSRF guard as webhooks, so a URL that resolves to a private address is not probed and reports unknown. A component whose status is unknown records no sample and is left out of uptime.

The uptime bars are built from the samples the control plane itself records while the page is enabled (one per component per `APP_STATUS_PAGE_SAMPLE_INTERVAL`, default `1m`). There is no backfill: days before you enabled the page show as "no data". A day's bar is red when at least 5 percent of its samples failed, yellow when some samples failed or were degraded, and green otherwise. Uptime is the share of samples that were not outages.

## Incidents and maintenance

Post an incident with an impact (`none`, `minor`, `major`, `critical`) and the affected components. Minor impact marks those components degraded and major or critical marks them as an outage, on top of what probes report, until the incident is resolved. Each update moves the incident through `investigating`, `identified`, `monitoring`, `resolved` and is added to its timeline.

Maintenance has a start, an end and a status of `scheduled`, `in_progress` or `completed`. Affected components show as "maintenance" while it is active.

```bash
levelrail-cli status-page incidents create --title "Slow payments" --impact minor --component sc_abc --body "We are **investigating** elevated latency."
levelrail-cli status-page incidents update si_123 --status resolved --body "Fixed at 14:05 UTC."
levelrail-cli status-page incidents create --kind maintenance --title "Database upgrade" --starts 2026-10-01T02:00:00Z --ends 2026-10-01T03:00:00Z
```

## Custom domain

Set a custom domain (`status-page set --domain status.example.com`) to serve the page at the root of that host. Route the domain to the control plane through your ingress and DNS first; this setting only makes the control plane answer for that Host. On that host only `/`, `/status.json` and `/status.rss` (and the `/public/status*` paths) are served, and every other path, including the dashboard and the API, returns 404.

## Operating it

- **Rate limited and cacheable.** Public reads are limited per client IP (`APP_STATUS_PAGE_RATE_LIMIT_RPM`, default `120`), the built view is cached (`APP_STATUS_PAGE_CACHE_TTL`, default `30s`), and responses carry `Cache-Control` and an `ETag`, so a CDN or browser can revalidate cheaply. Page responses carry a restrictive Content-Security-Policy (`default-src 'none'`, inline styles only).
- **No authentication on the public routes.** They are on the public allowlist, with a reason, in the authorization matrix test. Management routes under `/api/v1/status-page` need the `read` or `write` ability and are checked against the `status-page` resource, so an IAM policy can grant or deny them.
- **Sampling** only runs while the page is enabled. `APP_STATUS_PAGE_CHECK_TIMEOUT` (default `8s`) bounds each probe.
- **Other environment variables:** `APP_STATUS_PAGE_SAMPLE_INTERVAL`, `APP_STATUS_PAGE_CACHE_TTL`, `APP_STATUS_PAGE_CHECK_TIMEOUT`, `APP_STATUS_PAGE_RATE_LIMIT_RPM`.
- **Client IP for rate limiting** is the TCP peer address; `X-Forwarded-For` is not trusted. Behind a reverse proxy all visitors share the proxy's budget, so raise the limit or cache at the proxy.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/public/status`, `/public/status.json`, `/public/status.rss` | none (opt-in, rate limited) |
| `GET` | `/api/v1/status-page` | `read` |
| `PUT` | `/api/v1/status-page` | `write` |
| `GET` | `/api/v1/status-page/preview` | `read` |
| `GET` `POST` | `/api/v1/status-page/components` | `read`, `write` |
| `PUT` `DELETE` | `/api/v1/status-page/components/{id}` | `write` |
| `GET` `POST` | `/api/v1/status-page/incidents` | `read`, `write` |
| `POST` | `/api/v1/status-page/incidents/{id}/updates` | `write` |
| `DELETE` | `/api/v1/status-page/incidents/{id}` | `write` |

The `get_status_page` and `list_status_incidents` MCP tools are read only. Publishing incidents is deliberately not exposed to MCP, since it posts public text.

## Limits

- No subscriber notifications (email or webhook); the RSS feed is the push channel.
- Uptime is measured from this control plane, not from several regions, and covers only the time the page has been enabled.
- One page per control plane.
